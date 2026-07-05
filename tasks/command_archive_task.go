package tasks

import (
	"context"

	"github.com/dhamidi/k-si/runtime"
)

// "archive-task" — archive every workspace file, verify, then delete the workspace (archive-then-delete)
const ArchiveTask = "archive-task"

type ArchiveTaskPayload struct {
	TaskID int64 `json:"task_id"`
}

func NewArchiveTask(p ArchiveTaskPayload) runtime.Cmd {
	return runtime.NewCmd(ArchiveTask, p)
}

func registerArchiveTask(mod *runtime.Module) {
	runtime.HandleCmd(mod, ArchiveTask, archiveTaskEffect)
}

func archiveTaskEffect(ctx context.Context, e Edges, p ArchiveTaskPayload,
	emit runtime.Emit) error {
	return nil
}
