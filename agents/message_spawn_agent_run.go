package agents

import (
	"github.com/dhamidi/k-si/agents/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "spawn-agent-run" — sent by tasks; start (or resume) the worker harness for a task turn

func registerSpawnAgentRun(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.SpawnAgentRun, handleSpawnAgentRun)
}

func handleSpawnAgentRun(v runtime.View, s Model, p msg.SpawnAgentRunPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	return s, nil
}
