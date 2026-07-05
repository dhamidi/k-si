package email

import "github.com/dhamidi/k-si/runtime"

// "mark-email-sent" — mark an outbox row sent once the mail edge has transmitted it
const MarkEmailSent = "mark-email-sent"

type MarkEmailSentPayload struct {
	OutboxID int64 `json:"outbox_id"`
}

func NewMarkEmailSent(p MarkEmailSentPayload) runtime.Msg {
	return runtime.NewMsg(MarkEmailSent, p)
}

func registerMarkEmailSent(mod *runtime.Module) {
	runtime.HandleMsg(mod, MarkEmailSent, handleMarkEmailSent)
}

func handleMarkEmailSent(v runtime.View, s Model, p MarkEmailSentPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	next := append([]OutboxEntry(nil), s.Outbox...)
	for i := range next {
		if next[i].OutboxID == p.OutboxID {
			next[i].Status = "sent"
		}
	}
	s.Outbox = next
	return s, nil
}
