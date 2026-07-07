package tasks

import "github.com/dhamidi/k-si/runtime"

// "mark-harvested" — remove a HarvestJob once the capture-memory effect has
// emitted all its remember/forget directives (docs/03). It is the harvest's
// mark-email-sent: the pending marker clears only here, at the END of the effect,
// so a crash before it leaves the job pending for reconciliation to re-drive.
const MarkHarvested = "mark-harvested"

type MarkHarvestedPayload struct {
	RunID int64 `json:"run_id"`
}

func NewMarkHarvested(p MarkHarvestedPayload) runtime.Msg {
	return runtime.NewMsg(MarkHarvested, p)
}

func registerMarkHarvested(mod *runtime.Module) {
	runtime.HandleMsg(mod, MarkHarvested, handleMarkHarvested)
}

func handleMarkHarvested(v runtime.View, s Model, p MarkHarvestedPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// An absent RunID is a no-op, so a replay that re-folds mark-harvested — or a
	// second mark-harvested from a re-driven harvest — is idempotent (copy-on-write).
	s.HarvestPending = withoutHarvestPending(s.HarvestPending, p.RunID)
	return s, nil
}
