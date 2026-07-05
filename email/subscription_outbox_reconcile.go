package email

import (
	"context"
	"fmt"

	"github.com/dhamidi/k-si/runtime"
)

// outbox-reconcile — for every pending outbox row, keep emitting send-email until it is sent (crash-safe delivery)
//
// A pure function from state to the set of sources that should be running, each
// with a stable id; the runtime diffs and starts/stops them (docs/01). One
// source per pending outbox entry: it fires send-email once when the entry
// appears, and — because the entry disappears only when mark-email-sent flips it
// to "sent" — a crash that loses the in-flight send is recovered on restart,
// where replay rebuilds the pending entry and this source fires again. The
// pre-generated Message-ID keeps redelivery exactly-once (docs/03, docs/04).
func outboxReconcileSubs(v runtime.View, s Model) []runtime.Sub {
	var subs []runtime.Sub
	for _, entry := range s.Outbox {
		if entry.Status != "pending" {
			continue
		}
		entry := entry
		subs = append(subs, runtime.Sub{
			ID: fmt.Sprintf("send-email:%d", entry.OutboxID),
			// One-shot: emit send-outbox and finish. Await so the emit is drained
			// before the driving stimulus settles (docs/13), rather than racing it.
			Await: true,
			Run: func(ctx context.Context, edges any, emit runtime.Emit) {
				emit(NewSendOutbox(SendOutboxPayload{
					OutboxID:  entry.OutboxID,
					MessageID: entry.MessageID,
				}))
			},
		})
	}
	return subs
}
