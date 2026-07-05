package counter

import (
	"github.com/dhamidi/k-si/counter/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "increment-counter" — sent by scenarios, edges, and the web form; moves
// the count by the given amount. Contract message: the tag lives in msg/
// because other edges construct it (docs/15).

func registerIncrementCounter(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.IncrementCounter, handleIncrementCounter)
}

func handleIncrementCounter(v runtime.View, s Model, p msg.IncrementCounterPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	s.Count += p.By
	return s, nil
}
