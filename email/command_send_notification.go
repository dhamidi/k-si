package email

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/runtime"
)

// "send-notification" — send a threaded one-line notification mail, fire-and-forget (no outbox, no completion link)

func registerSendNotification(mod *runtime.Module) {
	runtime.HandleCmd(mod, msg.SendNotification, sendNotificationEffect)
}

// sendNotificationEffect builds a threaded one-line notification mail and submits
// it DIRECTLY through the mail edge — no outbox row, no reconcile, no completion
// link, no attachments. This is DELIBERATELY fire-and-forget, the inverse of the
// reply path's crash-safe outbox (decision-013): a notification is one-way and
// time-sensitive (a 2FA countdown), so a stale re-send after a restart is worse
// than a drop — better to lose it than deliver it late. On replay the effect is
// suppressed, so a folded log never re-sends. It emits nothing.
func sendNotificationEffect(ctx context.Context, e Edges, p msg.SendNotificationPayload,
	emit runtime.Emit) error {

	hdr := map[string][]string{
		"From":             {p.From},
		"To":               {strings.Join(p.To, ", ")},
		"Subject":          {p.Subject},
		"Message-ID":       {p.MessageID},
		"In-Reply-To":      {p.InReplyTo},
		"References":       {strings.Join(p.References, " ")},
		"X-Kasi-Task":      {strconv.FormatInt(p.TaskID, 10)},
		"X-Kasi-Agent-Run": {strconv.FormatInt(p.RunID, 10)},
		"X-Kasi-Notify":    {"1"},
	}

	raw, err := mime.Build(hdr, p.Body, nil) // body verbatim, no completion link, no attachments
	if err != nil {
		return fmt.Errorf("email: send-notification: build: %w", err)
	}
	return e.Mail.Submit(ctx, raw) // fire-and-forget
}
