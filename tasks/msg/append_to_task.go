package msg

import "github.com/dhamidi/k-si/runtime"

// "append-to-task" — sent by email/route-email; a participant's reply threads onto an existing task
const AppendToTask = "append-to-task"

type AppendToTaskPayload struct {
	TaskID    int64    `json:"task_id"`
	InboxID   int64    `json:"inbox_id"`
	Sender    string   `json:"sender"`
	Cc        []string `json:"cc"`
	Subject   string   `json:"subject"`
	MessageID string   `json:"message_id"`
	InReplyTo string   `json:"in_reply_to"`
}

func NewAppendToTask(p AppendToTaskPayload) runtime.Msg {
	return runtime.NewMsg(AppendToTask, p)
}
