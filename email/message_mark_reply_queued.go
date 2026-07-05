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

	return s, nil
}
