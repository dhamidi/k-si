package runtime

import "context"

// Sub is one declared, long-lived message source. A module's subscription
// provider is a pure function from state to the set of Subs that should be
// running; the runtime diffs the declared set against the running set by ID
// and starts/stops sources accordingly (docs/01).
type Sub struct {
	// ID is the stable identity, e.g. "agent-watch:task-42".
	ID string

	// Run is an edge-style body: edges and emit, never the model. It should
	// return when ctx is cancelled.
	Run func(ctx context.Context, edges any, emit Emit)

	// Await marks a one-shot source the runtime must treat as pending work
	// until its Run returns — so a reconciliation source that emits a message
	// and finishes is counted before quiescence, rather than racing the
	// stimulus's Settle (docs/03, docs/13). Leave false for long-lived watchers
	// and pollers that block until an external event: a blocking source that is
	// merely waiting IS a stable, quiescent state (docs/14), and awaiting it
	// would wrongly keep the instance forever busy.
	Await bool
}

type runningSub struct {
	cancel context.CancelFunc
	done   chan struct{}
}
