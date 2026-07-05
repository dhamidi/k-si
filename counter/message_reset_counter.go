package counter

import "github.com/dhamidi/k-si/runtime"

// "reset-counter" — asks for a reset; the actual zeroing crosses the send boundary
const ResetCounter = "reset-counter"

type ResetCounterPayload struct{}

func NewResetCounter(p ResetCounterPayload) runtime.Msg {
	return runtime.NewMsg(ResetCounter, p)
}

func registerResetCounter(mod *runtime.Module) {
	runtime.HandleMsg(mod, ResetCounter, handleResetCounter)
}

// The hand-off exists so the canary exercises the send command and its trace
// rendering (send:mark-counter-reset) end to end (docs/01).
func handleResetCounter(v runtime.View, s Model, p ResetCounterPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	return s, []runtime.Cmd{
		runtime.Send(NewMarkCounterReset(MarkCounterResetPayload{Previous: s.Count})),
	}
}
