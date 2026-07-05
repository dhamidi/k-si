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
}

func NewStartAgentRun(p StartAgentRunPayload) runtime.Cmd {
	return runtime.NewCmd(StartAgentRun, p)
}

func registerStartAgentRun(mod *runtime.Module) {
	runtime.HandleCmd(mod, StartAgentRun, startAgentRunEffect)
}

func startAgentRunEffect(ctx context.Context, e Edges, p StartAgentRunPayload,
	emit runtime.Emit) error {
	// Register the live run and return immediately; the agent-watch
	// subscription emits finish-agent-run when the turn completes (docs/05).
	// No emit here — results leave only via that subscription.
	var err error
	if p.Resume {
		_, err = e.Harness.Resume(ctx, p.TaskID, p.RunID, sessionFor(p.TaskID))
	} else {
		_, err = e.Harness.Start(ctx, p.TaskID, p.RunID)
	}
	return err
}
