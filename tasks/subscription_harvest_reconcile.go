package tasks

import (
	"context"
	"fmt"

	"github.com/dhamidi/k-si/runtime"
)

// harvest-reconcile — for every still-pending HarvestJob, keep driving the memory
// harvest until it completes (crash-safe harvest, the memory sibling of email's
// outbox reconcile, docs/03).
//
// A pure function from state to the sources that should be running, each with a
// stable id; the runtime diffs the declared set against the running set and
// starts/stops sources (docs/01). One source per pending job: it emits run-harvest
// once when the job appears, and — because the job disappears only when
// mark-harvested removes it (emitted at the END of the capture-memory effect) — a
// crash that loses the in-flight harvest is recovered on restart, where replay
// rebuilds the pending job and this source fires again. remember (upsert) and
// forget (no-op-if-absent) are idempotent, so re-running the whole harvest is safe.
func harvestReconcileSubs(v runtime.View, s Model) []runtime.Sub {
	var subs []runtime.Sub
	for _, job := range s.HarvestPending {
		job := job
		subs = append(subs, runtime.Sub{
			ID: fmt.Sprintf("capture-memory:%d", job.RunID),
			// One-shot: emit run-harvest and finish. Await so the emit — and the
			// capture-memory effect it drives, ending in mark-harvested — is drained
			// before the driving stimulus settles (docs/13), rather than racing it.
			Await: true,
			Run: func(ctx context.Context, edges any, emit runtime.Emit) {
				emit(NewRunHarvest(RunHarvestPayload{
					TaskID: job.TaskID,
					RunID:  job.RunID,
				}))
			},
		})
	}
	return subs
}
