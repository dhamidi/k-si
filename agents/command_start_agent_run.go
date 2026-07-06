package agents

import (
	"context"

	"github.com/dhamidi/k-si/runtime"
)

// "start-agent-run" — start or resume the worker harness in the task workspace
const StartAgentRun = "start-agent-run"

type StartAgentRunPayload struct {
	TaskID int64 `json:"task_id"`
	RunID  int64 `json:"run_id"`
	Resume bool  `json:"resume"`
	// SecretRefs maps an env-var name to a secret:// URL, resolved into the run
	// environment at the harness edge (Flow C, docs/06). Carry-through only here.
	SecretRefs map[string]string `json:"secret_refs"`
}

func NewStartAgentRun(p StartAgentRunPayload) runtime.Cmd {
	return runtime.NewCmd(StartAgentRun, p)
}

func registerStartAgentRun(mod *runtime.Module) {
	runtime.HandleCmd(mod, StartAgentRun, startAgentRunEffect)
}

func startAgentRunEffect(ctx context.Context, e Edges, p StartAgentRunPayload,
	emit runtime.Emit) error {
	// Resolve any requested secrets into the run environment at the edge (Flow C,
	// decision-004): the message carried only secret:// references, plaintext is
	// materialised here and nowhere else. Keyed by the request field name.
	var env map[string]string
	for field, url := range p.SecretRefs {
		plaintext, err := e.Secrets.Resolve(ctx, url)
		if err != nil {
			return err
		}
		if env == nil {
			env = make(map[string]string, len(p.SecretRefs))
		}
		env[field] = plaintext
	}

	// Register the live run and return immediately; the agent-watch
	// subscription emits finish-agent-run when the turn completes (docs/05).
	// No emit here — results leave only via that subscription.
	var err error
	if p.Resume {
		_, err = e.Harness.Resume(ctx, p.TaskID, p.RunID, sessionFor(p.TaskID), env)
	} else {
		_, err = e.Harness.Start(ctx, p.TaskID, p.RunID, env)
	}
	return err
}
