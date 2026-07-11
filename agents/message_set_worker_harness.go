package agents

import (
	"github.com/dhamidi/k-si/agents/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "set-worker-harness" — record the operator's chosen worker harness in the model
// (decision-024). The spawn handler reads it to pin each FRESH run; a run already
// pinned keeps its harness for its whole life.

func registerSetWorkerHarness(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.SetWorkerHarness, handleSetWorkerHarness)
}

func handleSetWorkerHarness(v runtime.View, s Model, p msg.SetWorkerHarnessPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	s.WorkerHarness = p.Name
	return s, nil
}
