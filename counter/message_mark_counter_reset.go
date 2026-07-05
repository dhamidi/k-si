package counter

import "github.com/dhamidi/k-si/runtime"

// "mark-counter-reset" — zeroes the count; sent by reset-counter via the send command
const MarkCounterReset = "mark-counter-reset"

type MarkCounterResetPayload struct {
	Previous int64 `json:"previous"`
}

func NewMarkCounterReset(p MarkCounterResetPayload) runtime.Msg {
	return runtime.NewMsg(MarkCounterReset, p)
}

func registerMarkCounterReset(mod *runtime.Module) {
	runtime.HandleMsg(mod, MarkCounterReset, handleMarkCounterReset)
}

func handleMarkCounterReset(v runtime.View, s Model, p MarkCounterResetPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	s.Count = 0
	return s, nil
}
