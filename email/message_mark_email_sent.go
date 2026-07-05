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

	return s, nil
}
