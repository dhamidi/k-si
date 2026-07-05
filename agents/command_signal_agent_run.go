package agents

import (
	"context"

	"github.com/dhamidi/k-si/runtime"
)

// "signal-agent-run" — signal the harness process to terminate (graceful, then hard)
const SignalAgentRun = "signal-agent-run"

type SignalAgentRunPayload struct {
	TaskID int64 `json:"task_id"`
	RunID  int64 `json:"run_id"`
}

func NewSignalAgentRun(p SignalAgentRunPayload) runtime.Cmd {
	return runtime.NewCmd(SignalAgentRun, p)
}

func registerSignalAgentRun(mod *runtime.Module) {
	runtime.HandleCmd(mod, SignalAgentRun, signalAgentRunEffect)
}

func signalAgentRunEffect(ctx context.Context, e Edges, p SignalAgentRunPayload,
	emit runtime.Emit) error {
	return nil
}
