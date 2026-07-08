package msg

import "github.com/dhamidi/k-si/runtime"

// "append-to-task" — sent by email/route-email; a participant's reply threads onto an existing task
const AppendToTask = "append-to-task"

type AppendToTaskPayload struct {
	TaskID  int64  `json:"task_id"`
	InboxID int64  `json:"inbox_id"`
	Sender  string `json:"sender"`
	// To and Cc are the message's other recipients; both join the participant set
	// (minus käsi's own addresses), so a multi-party thread reply-alls to everyone
	// (multiplayer, decision-017). To is absent on pre-decision-017 log entries, which
	// decode it as nil — the old From+Cc participants — so replay stays convergent.
	To        []string `json:"to"`
	Cc        []string `json:"cc"`
	Subject   string   `json:"subject"`
	MessageID string   `json:"message_id"`
	InReplyTo string   `json:"in_reply_to"`
}

func NewAppendToTask(p AppendToTaskPayload) runtime.Msg {
	return runtime.NewMsg(AppendToTask, p)
}
