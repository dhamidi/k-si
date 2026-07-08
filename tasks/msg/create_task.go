package msg

import "github.com/dhamidi/k-si/runtime"

// "create-task" — sent by email/route-email; creates the Task and seeds participants
const CreateTask = "create-task"

type CreateTaskPayload struct {
	InboxID  int64  `json:"inbox_id"`
	Route    string `json:"route"`
	Template string `json:"template"`
	Sender   string `json:"sender"`
	// To and Cc are the message's other recipients; both seed the participant set
	// (minus käsi's own addresses) so käsi reply-alls a multi-party thread
	// (multiplayer, decision-017). To is nil on pre-decision-017 log entries, giving
	// the old From+Cc participants, so replay stays convergent.
	To        []string `json:"to"`
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
