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
	s.Runs[i].Status = StatusStopping
	return s, []runtime.Cmd{
		NewSignalAgentRun(SignalAgentRunPayload{TaskID: p.TaskID, RunID: p.RunID}),
	}
}
