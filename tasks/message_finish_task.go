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

	i := s.find(TaskID(p.TaskID))
	if i < 0 {
		return s, nil
	}

	tasks := append([]Task(nil), s.Tasks...)
	t := tasks[i]
	t.Status = Done
	tasks[i] = t
	s.Tasks = tasks

	return s, []runtime.Cmd{
		NewArchiveTask(ArchiveTaskPayload{TaskID: p.TaskID}),
	}
}
