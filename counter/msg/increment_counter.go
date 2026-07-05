package msg

import "github.com/dhamidi/k-si/runtime"

// "increment-counter" — sent by scenarios, edges, and the web form; moves the count by the given amount
const IncrementCounter = "increment-counter"

type IncrementCounterPayload struct {
	By int64 `json:"by"`
}

func NewIncrementCounter(p IncrementCounterPayload) runtime.Msg {
	return runtime.NewMsg(IncrementCounter, p)
}
