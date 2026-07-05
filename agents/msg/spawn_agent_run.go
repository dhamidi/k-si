package msg

import "github.com/dhamidi/k-si/runtime"

// "spawn-agent-run" — sent by tasks; start (or resume) the worker harness for a task turn
const SpawnAgentRun = "spawn-agent-run"

type SpawnAgentRunPayload struct {
	TaskID int64 `json:"task_id"`
	Resume bool  `json:"resume"`
}

func NewSpawnAgentRun(p SpawnAgentRunPayload) runtime.Msg {
	return runtime.NewMsg(SpawnAgentRun, p)
}
