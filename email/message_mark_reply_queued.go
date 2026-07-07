package email

import "github.com/dhamidi/k-si/runtime"

// "mark-reply-queued" — record a pending outbox row so reconciliation will send it
const MarkReplyQueued = "mark-reply-queued"

type MarkReplyQueuedPayload struct {
	TaskID    int64  `json:"task_id"`
	OutboxID  int64  `json:"outbox_id"`
	MessageID string `json:"message_id"`
	InReplyTo string `json:"in_reply_to"`
}

func NewMarkReplyQueued(p MarkReplyQueuedPayload) runtime.Msg {
	return runtime.NewMsg(MarkReplyQueued, p)
}

func registerMarkReplyQueued(mod *runtime.Module) {
	runtime.HandleMsg(mod, MarkReplyQueued, handleMarkReplyQueued)
}

func handleMarkReplyQueued(v runtime.View, s Model, p MarkReplyQueuedPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// Dedup: a re-driven assemble-reply (the reply harvest re-firing after a crash)
	// hands AddOutbox the same deterministic Message-ID, which returns the EXISTING
	// row id — so this OutboxID/MessageID may already be pending here. Appending it
	// again would declare a second send source for one row. Skip the duplicate so
	// reconciliation stays exactly-once (decision-013).
	for _, e := range s.Outbox {
		if e.OutboxID == p.OutboxID || (p.MessageID != "" && e.MessageID == p.MessageID) {
			return s, nil
		}
	}

	// Record the queued reply as pending. The outbox-reconcile subscription then
	// sees a pending entry and emits send-email — the single send path, so a
	// crash that loses the in-flight send is recovered by replay rebuilding this
	// entry and reconciliation firing again (docs/03).
	s.Outbox = append(append([]OutboxEntry(nil), s.Outbox...), OutboxEntry{
		OutboxID:  p.OutboxID,
		TaskID:    p.TaskID,
		MessageID: p.MessageID,
		Status:    "pending",
	})
	return s, nil
}
