package agents

import (
	"github.com/dhamidi/k-si/agents/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "stop-agent-run" — sent by the web Stop button or the supervisor; stop a running harness

func registerStopAgentRun(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.StopAgentRun, handleStopAgentRun)
}

func handleStopAgentRun(v runtime.View, s Model, p msg.StopAgentRunPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	i := s.findRun(AgentRunID(p.RunID))
	if i < 0 {
		return s, nil
	}
	runs := append([]AgentRun(nil), s.Runs...) // copy-on-write: never mutate the shared snapshot
	runs[i].Status = StatusStopping
	s.Runs = runs
	return s, []runtime.Cmd{
		// Carry the run's pinned harness + session so the effect signals the SAME
		// harness that launched it (decision-024).
		NewSignalAgentRun(SignalAgentRunPayload{
			TaskID:  p.TaskID,
			RunID:   p.RunID,
			Harness: runs[i].Harness,
			Session: runs[i].Session,
		}),
	}
}
