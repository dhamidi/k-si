package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"
)

// TaskView is the data view_task.vue renders — one task's detail (docs/08): it
// leads with status + subject + participants, then the agent runs (each with a
// transcript link and, for the active run, a Stop form), then any open UI
// request, then the archived artifacts. Built from the model + the content store
// by the route handler, never a raw model object (docs/08, docs/15).
type TaskView struct {
	ID           int64
	Status       string
	Subject      string
	Route        string
	Participants []string
	Runs         []RunRow
	// Request is the open UI request an agent raised, if any — a link the
	// operator can follow to answer it (Flow C). Empty URL means none is open.
	Request RequestLink
	// Artifacts are the filenames archived for this task (docs/08).
	Artifacts []string
	// Nav is the shared top-level navbar (navView) — a task detail lights the
	// Tasks section.
	Nav NavView
}

// RunRow is one agent run in the detail view: its number, the path to its
// transcript, and — only for the active run — the Stop form's POST target
// (docs/08, "Stopping an agent"). Paths are reverse-routed, never string-built.
type RunRow struct {
	Number         int64
	TranscriptPath string
	Active         bool
	StopPath       string
}

// RequestLink points at an open UI request's capability link (the tokened
// request route). Present is false when the task has no open request.
type RequestLink struct {
	Present bool
	URL     string
}

// RenderTask writes the full task-detail page (docs/08).
func RenderTask(ctx context.Context, w io.Writer, engine *htmlc.Engine, view TaskView) error {
	return engine.RenderPage(ctx, w, "view_task", map[string]any{
		"task": view,
	})
}
