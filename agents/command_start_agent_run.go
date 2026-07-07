package agents

import (
	"context"
	"fmt"
	"log"

	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/skilltree"
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

	// Provision every learned skill into this run's workspace before the harness
	// starts — the single choke point every run passes through, so skills authored
	// by any task are laid into every future run by default (Flow D, decision-009).
	if err := provisionSkills(e, p.TaskID); err != nil {
		return err
	}

	// Symlink the persistent store into this run's workspace at ./store/ — the
	// same spawn choke point that provisions skills (Flow F, decision-012). The
	// store lives outside the workspace and the event log; the link makes it live
	// for the agent, and archival skips it so completing a task never touches it.
	if err := e.Store.Link(p.TaskID); err != nil {
		return err
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

// provisionSkills lays every skill in the registry into the run's workspace
// skills/<name>/ box (docs/07 provisioning). It reads the skill table directly —
// an effect has no model — and unpacks each skill's tar tree, rooting it under the
// skill name. A corrupt blob is logged and skipped rather than blocking the run.
func provisionSkills(e Edges, taskID int64) error {
	rows, err := e.Content.AllSkills()
	if err != nil {
		return fmt.Errorf("agents: provision skills: %w", err)
	}
	for _, row := range rows {
		parts, err := skilltree.Unpack(row.Content)
		if err != nil {
			log.Printf("agents: provision skill %q: %v", row.Name, err)
			continue
		}
		rooted := make([]mime.Part, 0, len(parts))
		for _, part := range parts {
			part.Filename = row.Name + "/" + part.Filename
			rooted = append(rooted, part)
		}
		if err := e.Work.WriteSkills(taskID, rooted); err != nil {
			return fmt.Errorf("agents: provision skill %q: %w", row.Name, err)
		}
	}
	return nil
}
