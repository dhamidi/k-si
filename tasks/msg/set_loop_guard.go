package msg

import "github.com/dhamidi/k-si/runtime"

// "set-loop-guard" — set the per-task run cap that trips the loop breaker (decision-016)
const SetLoopGuard = "set-loop-guard"

type SetLoopGuardPayload struct {
	Max int `json:"max"`
}

func NewSetLoopGuard(p SetLoopGuardPayload) runtime.Msg {
	return runtime.NewMsg(SetLoopGuard, p)
}
