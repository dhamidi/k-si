package msg

import "github.com/dhamidi/k-si/runtime"

// "create-task" — sent by email/route-email; creates the Task and seeds participants
const CreateTask = "create-task"

type CreateTaskPayload struct {
	InboxID   int64    `json:"inbox_id"`
	Route     string   `json:"route"`
	Template  string   `json:"template"`
	Sender    string   `json:"sender"`
	Cc        []string `json:"cc"`
	Subject   string   `json:"subject"`
	MessageID string   `json:"message_id"`
	// CompletionToken guards the task's completion link — minted at the inbound
	// edge (unguessable in production), carried here rather than derived from the
	// task id (docs/04, docs/13).
	CompletionToken string `json:"completion_token"`
}

func NewCreateTask(p CreateTaskPayload) runtime.Msg {
	return runtime.NewMsg(CreateTask, p)
}
