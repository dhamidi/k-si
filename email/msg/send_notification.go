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
}

func NewSendNotification(p SendNotificationPayload) runtime.Cmd {
	return runtime.NewCmd(SendNotification, p)
}
