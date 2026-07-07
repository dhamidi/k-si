package tasks

import (
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/tasks/msg"
)

// "mark-harvested" — remove the (RunID, Kind) HarvestJob once its post-finish
// effect has emitted all its logged directives (decision-013). The message and its
// payload live in the tasks contract package so the reply harvest's email-side
// effect can clear its job cross-module; this is its reducer.

func registerMarkHarvested(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.MarkHarvested, handleMarkHarvested)
}

func handleMarkHarvested(v runtime.View, s Model, p msg.MarkHarvestedPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// An absent (RunID, Kind) is a no-op, so a replay that re-folds mark-harvested —
	// or a second mark-harvested from a re-driven harvest — is idempotent
	// (copy-on-write). Only the matching kind clears; the run's other pending kinds
	// stay owed.
	s.HarvestPending = withoutHarvestPending(s.HarvestPending, p.RunID, p.Kind)
	return s, nil
}
