package counter

import "github.com/dhamidi/k-si/runtime"

// "increment-counter" — sent by scenarios and edges that count; moves the count by the given amount
const IncrementCounter = "increment-counter"

type IncrementCounterPayload struct {
	By int64 `json:"by"`
}

func NewIncrementCounter(p IncrementCounterPayload) runtime.Msg {
	return runtime.NewMsg(IncrementCounter, p)
}

func registerIncrementCounter(mod *runtime.Module) {
	runtime.HandleMsg(mod, IncrementCounter, handleIncrementCounter)
}

func handleIncrementCounter(v runtime.View, s Model, p IncrementCounterPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	s.Count += p.By
	return s, nil
}
