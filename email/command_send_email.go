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
	// Mechanism is the name of the sender to submit through, resolved from the live
	// OutboundVia in the send-outbox handler so the effect stays model-blind
	// (decision-023). Absent on pre-decision-023 payloads, which never run on replay.
	Mechanism string `json:"mechanism,omitempty"`
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
	// Dispatch to the mechanism the handler chose. An unknown or unconfigured
	// mechanism (disabled sender, or a name with no built edge) returns an error so
	// the row stays pending and is retried on the next restart — never silently
	// dropped (decision-023, decision-016). The lookup is the whole model-blindness
	// of the effect: it never reads the view, only the name in the payload.
	mail, ok := e.Senders[p.Mechanism]
	if !ok {
		return fmt.Errorf("email: send-email: no sender for mechanism %q (outbox %d left pending)", p.Mechanism, p.OutboxID)
	}
	// A send failure is left to reconciliation: the row stays pending, and the
	// pre-generated Message-ID makes a later resend a duplicate the provider
	// drops (docs/03, docs/04). We do NOT retry inside the effect.
	if err := mail.Submit(ctx, row.Raw); err != nil {
		return err
	}
	if err := e.Content.MarkOutboxSent(p.OutboxID, e.Clock.Now()); err != nil {
		return err
	}
	emit(NewMarkEmailSent(MarkEmailSentPayload{OutboxID: p.OutboxID}))
	return nil
}
