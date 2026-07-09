package apps

import (
	"context"
	"fmt"

	"github.com/dhamidi/k-si/runtime"
)

// apps-reconcile — for each registered app not yet running, emit run-app; for each removing app, emit retire-app (crash-safe, decision-013)
//
// A pure function from state to the set of sources that should be running,
// each with a stable id; the runtime diffs and starts/stops them (docs/01).
// One source per app whose unit doesn't yet match its desired Status, exactly
// like outbox-reconcile (email/subscription_outbox_reconcile.go). Status only
// leaves "registered"/"removing" when mark-app-running/mark-app-removed
// record the effect's success in the log, so a crash that loses an in-flight
// systemctl is rebuilt by replay and the source fires again — and
// Runner.Install/Remove are idempotent, so the re-fire is safe (docs/13).
func appsReconcileSubs(v runtime.View, s Model) []runtime.Sub {
	var subs []runtime.Sub
	for _, app := range s.Apps {
		switch app.Status {
		case StatusRegistered:
			name := app.Name
			subs = append(subs, runtime.Sub{
				ID: fmt.Sprintf("run-app:%s", name),
				// One-shot: emit run-app and finish. Await so the emit is drained
				// before the driving stimulus settles (docs/13), rather than racing
				// it.
				Await: true,
				Run: func(ctx context.Context, edges any, emit runtime.Emit) {
					emit(NewRunApp(RunAppPayload{Name: name}))
				},
			})
		case StatusRemoving:
			name := app.Name
			subs = append(subs, runtime.Sub{
				ID:    fmt.Sprintf("retire-app:%s", name),
				Await: true,
				Run: func(ctx context.Context, edges any, emit runtime.Emit) {
					emit(NewRetireApp(RetireAppPayload{Name: name}))
				},
			})
		}
	}
	return subs
}
