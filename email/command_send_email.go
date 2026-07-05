package email

import (
	"context"
	"fmt"

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

	row, err := e.Content.Outbox(p.OutboxID)
	if err != nil {
		return fmt.Errorf("email: send-email: load outbox %d: %w", p.OutboxID, err)
	}
	// A send failure is left to reconciliation: the row stays pending, and the
	// pre-generated Message-ID makes a later resend a duplicate the provider
	// drops (docs/03, docs/04). We do NOT retry inside the effect.
	if err := e.Mail.Submit(ctx, row.Raw); err != nil {
		return err
	}
	if err := e.Content.MarkOutboxSent(p.OutboxID, e.Clock.Now()); err != nil {
		return err
	}
	emit(NewMarkEmailSent(MarkEmailSentPayload{OutboxID: p.OutboxID}))
	return nil
}
