package email

import "github.com/dhamidi/k-si/runtime"

// "send-outbox" — reconciliation asks to transmit a pending outbox row; the handler returns the send-email command
const SendOutbox = "send-outbox"

type SendOutboxPayload struct {
	OutboxID  int64  `json:"outbox_id"`
	MessageID string `json:"message_id"`
}

func NewSendOutbox(p SendOutboxPayload) runtime.Msg {
	return runtime.NewMsg(SendOutbox, p)
}

func registerSendOutbox(mod *runtime.Module) {
	runtime.HandleMsg(mod, SendOutbox, handleSendOutbox)
}

func handleSendOutbox(v runtime.View, s Model, p SendOutboxPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// A subscription cannot return a command, so it emits this message and the
	// handler turns it into the send-email effect (docs/01, docs/03). The active
	// sender is resolved HERE, from the live model — not at reconcile-sub build
	// time, which would freeze a stale choice — so switching the sender takes
	// effect on the next queued reply (decision-023).
	return s, []runtime.Cmd{NewSendEmail(SendEmailPayload{
		OutboxID:  p.OutboxID,
		MessageID: p.MessageID,
		Mechanism: OutboundVia(v),
	})}
}
