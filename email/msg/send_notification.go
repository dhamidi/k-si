package msg

import "github.com/dhamidi/k-si/runtime"

// "send-notification" — send a threaded one-line notification mail, fire-and-forget (no outbox, no completion link)
const SendNotification = "send-notification"

type SendNotificationPayload struct {
	To         []string `json:"to"`
	From       string   `json:"from"`
	Subject    string   `json:"subject"`
	InReplyTo  string   `json:"in_reply_to"`
	References []string `json:"references"`
	MessageID  string   `json:"message_id"`
	Body       string   `json:"body"`
	TaskID     int64    `json:"task_id"`
	RunID      int64    `json:"run_id"`
	// Mechanism is the sender to submit through — the active OutboundVia, resolved
	// by the emitting handler so a notification leaves through the same provider as
	// replies (decision-023). Empty means the spool default.
	Mechanism string `json:"mechanism,omitempty"`
}

func NewSendNotification(p SendNotificationPayload) runtime.Cmd {
	return runtime.NewCmd(SendNotification, p)
}
