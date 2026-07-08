package msg

import "github.com/dhamidi/k-si/runtime"

// "set-max-concurrent-runs" — cap concurrent live agent runs; the rest queue (decision-016)
const SetMaxConcurrentRuns = "set-max-concurrent-runs"

type SetMaxConcurrentRunsPayload struct {
	Max int `json:"max"`
}

func NewSetMaxConcurrentRuns(p SetMaxConcurrentRunsPayload) runtime.Msg {
	return runtime.NewMsg(SetMaxConcurrentRuns, p)
}
