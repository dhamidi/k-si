package email

import (
	"context"

	"github.com/dhamidi/k-si/runtime"
)

// "send-email" — transmit one pending outbox row via the mail edge; idempotent on Message-ID
const SendEmail = "send-email"

type SendEmailPayload struct {
	OutboxID  int64  `json:"outbox_id"`
	MessageID string `json:"message_id"`
}

func NewSendEmail(p SendEmailPayload) runtime.Cmd {
	return runtime.NewCmd(SendEmail, p)
}

func registerSendEmail(mod *runtime.Module) {
	runtime.HandleCmd(mod, SendEmail, sendEmailEffect)
}

func sendEmailEffect(ctx context.Context, e Edges, p SendEmailPayload,
	emit runtime.Emit) error {
	// On success, the result enters the model as a message (docs/01):
	// emit(NewMarkEmailSent(…))
	return nil
}
