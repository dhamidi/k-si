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
	// Harness and Session name the run's pinned harness and session (decision-024).
	// The effect has no View, so the stop handler copies them off the run; the effect
	// signals the SAME harness that launched, addressing the SAME session.
	Harness string `json:"harness,omitempty"`
	Session string `json:"session,omitempty"`
}

func NewSignalAgentRun(p SignalAgentRunPayload) runtime.Cmd {
	return runtime.NewCmd(SignalAgentRun, p)
}

func registerSignalAgentRun(mod *runtime.Module) {
	runtime.HandleCmd(mod, SignalAgentRun, signalAgentRunEffect)
}

func signalAgentRunEffect(ctx context.Context, e Edges, p SignalAgentRunPayload,
	emit runtime.Emit) error {
	// Ask the harness to stop; the agent-watch subscription observes the exit
	// (its ctx is also cancelled as the run leaves "running") and emits
	// finish-agent-run flagged stopped (docs/05). No emit here.
	session := p.Session
	if session == "" {
		session = sessionFor(p.TaskID)
	}
	return e.resolveHarness(p.Harness).Signal(ctx, Handle{TaskID: p.TaskID, RunID: p.RunID, Session: session})
}
