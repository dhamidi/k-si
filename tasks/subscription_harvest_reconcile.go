package tasks

import (
	"context"
	"fmt"

	"github.com/dhamidi/k-si/runtime"
)

// harvest-reconcile — for every still-pending HarvestJob, keep driving that kind's
// post-finish effect until it completes (crash-safe post-finish work, the sibling
// of email's outbox reconcile, docs/03; decision-013).
//
// A pure function from state to the sources that should be running, each with a
// stable id; the runtime diffs the declared set against the running set and
// starts/stops sources (docs/01). One source per pending (RunID, Kind) job: it
// emits run-harvest once when the job appears, and — because the job disappears
// only when mark-harvested removes it (emitted at the END of the effect that kind
// drives) — a crash that loses the in-flight effect is recovered on restart, where
// replay rebuilds the pending job and this source fires again. Every kind's effect
// emits idempotently (memory remember/forget, store-skill upsert, assemble-reply's
// deterministic Message-ID + idempotent AddOutbox, capture-transcript's
// content-addressed AddArchive keyed on (task_id, filename)), so re-running the
// whole effect is safe.
func harvestReconcileSubs(v runtime.View, s Model) []runtime.Sub {
	var subs []runtime.Sub
	for _, job := range s.HarvestPending {
		job := job
		subs = append(subs, runtime.Sub{
			// Kind is part of the id so a run's independent kinds each get their own
			// source (a run may owe a skill AND a reply at once).
			ID: fmt.Sprintf("harvest:%s:%d", job.Kind, job.RunID),
			// One-shot: emit run-harvest and finish. Await so the emit — and the
			// effect it drives, ending in mark-harvested — is drained before the
			// driving stimulus settles (docs/13), rather than racing it.
			Await: true,
			Run: func(ctx context.Context, edges any, emit runtime.Emit) {
				emit(NewRunHarvest(RunHarvestPayload{
					TaskID: job.TaskID,
					RunID:  job.RunID,
					Kind:   job.Kind,
				}))
			},
		})
	}
	return subs
}
