package tasks

import (
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/tasks/msg"
)

// "set-loop-guard" — set the per-task run cap that trips the loop breaker (decision-016)

func registerSetLoopGuard(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.SetLoopGuard, handleSetLoopGuard)
}

func handleSetLoopGuard(v runtime.View, s Model, p msg.SetLoopGuardPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	s.LoopGuard = p.Max
	return s, nil
}
