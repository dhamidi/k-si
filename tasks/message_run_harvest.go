package tasks

import "github.com/dhamidi/k-si/runtime"

// "run-harvest" — the harvest-reconcile subscription's trigger. A subscription
// cannot return a command (its emit takes a Msg, not a Cmd), so — exactly as
// email's send-outbox does for send-email (docs/03) — the reconcile source emits
// this message and its handler turns it into the capture-memory effect. One
// run-harvest per still-pending HarvestJob, until mark-harvested clears the job.
const RunHarvest = "run-harvest"

type RunHarvestPayload struct {
	TaskID int64 `json:"task_id"`
	RunID  int64 `json:"run_id"`
}

func NewRunHarvest(p RunHarvestPayload) runtime.Msg {
	return runtime.NewMsg(RunHarvest, p)
}

func registerRunHarvest(mod *runtime.Module) {
	runtime.HandleMsg(mod, RunHarvest, handleRunHarvest)
}

func handleRunHarvest(v runtime.View, s Model, p RunHarvestPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// A subscription cannot return a command, so it emits this message and the
	// handler turns it into the capture-memory effect (docs/01, docs/03).
	return s, []runtime.Cmd{NewCaptureMemory(CaptureMemoryPayload{
		TaskID: p.TaskID,
		RunID:  p.RunID,
	})}
}
