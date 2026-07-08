package msg

import "github.com/dhamidi/k-si/runtime"

// "notify-user" — injected by the control endpoint; email the task initiator a mid-run one-liner
const NotifyUser = "notify-user"

type NotifyUserPayload struct {
	TaskID int64  `json:"task_id"`
	RunID  int64  `json:"run_id"`
	Body   string `json:"body"`
}

func NewNotifyUser(p NotifyUserPayload) runtime.Msg {
	return runtime.NewMsg(NotifyUser, p)
}
