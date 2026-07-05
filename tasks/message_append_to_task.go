package tasks

import (
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/tasks/msg"
)

// "append-to-task" — sent by email/route-email; a participant's reply threads onto an existing task

func registerAppendToTask(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.AppendToTask, handleAppendToTask)
}

func handleAppendToTask(v runtime.View, s Model, p msg.AppendToTaskPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	return s, nil
}
