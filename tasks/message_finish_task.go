package tasks

import (
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/tasks/msg"
)

// "finish-task" — sent by the web completion link; archive-then-delete and mark the task done

func registerFinishTask(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.FinishTask, handleFinishTask)
}

func handleFinishTask(v runtime.View, s Model, p msg.FinishTaskPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	return s, nil
}
