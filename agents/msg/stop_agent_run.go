package msg

import "github.com/dhamidi/k-si/runtime"

// "stop-agent-run" — sent by the web Stop button or the supervisor; stop a running harness
const StopAgentRun = "stop-agent-run"

type StopAgentRunPayload struct {
	TaskID int64 `json:"task_id"`
	RunID  int64 `json:"run_id"`
}

func NewStopAgentRun(p StopAgentRunPayload) runtime.Msg {
	return runtime.NewMsg(StopAgentRun, p)
}
