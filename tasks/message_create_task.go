package tasks

import (
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/tasks/msg"
)

// "create-task" — sent by email/route-email; creates the Task and seeds participants

func registerCreateTask(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.CreateTask, handleCreateTask)
}

func handleCreateTask(v runtime.View, s Model, p msg.CreateTaskPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	return s, nil
}
