package email

import "github.com/dhamidi/k-si/runtime"

// outbox-reconcile — for every pending outbox row, keep emitting send-email until it is sent (crash-safe delivery)
//
// A pure function from state to the set of sources that should be running,
// each with a stable id; the runtime diffs and starts/stops them (docs/01).
func outboxReconcileSubs(v runtime.View, s Model) []runtime.Sub {
	return nil
}
