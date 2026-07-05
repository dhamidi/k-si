package msg

import "github.com/dhamidi/k-si/runtime"

// "finish-task" — sent by the web completion link; archive-then-delete and mark the task done
const FinishTask = "finish-task"

type FinishTaskPayload struct {
	TaskID int64 `json:"task_id"`
}

func NewFinishTask(p FinishTaskPayload) runtime.Msg {
	return runtime.NewMsg(FinishTask, p)
}
