package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"

	"github.com/dhamidi/k-si/tasks"
)

// TasksView is the data view_tasks.vue renders — the task list (docs/08): every
// task grouped by lifecycle status, newest-first within each group, empty groups
// omitted. Built from tasks.All by the route handler, never a raw model object
// (docs/08, docs/15). Single column, mobile-first, structure before style
// (decision-005).
type TasksView struct {
	Groups []TaskGroup
	// SkillsPath links to the skills registry (the small cross-nav both list
	// pages carry). Reverse-routed, never string-built (rule no-url-string-building).
	SkillsPath string
}

// TaskGroup is one status bucket of the list: a heading plus its rows, newest
// first. Only non-empty groups are built.
type TaskGroup struct {
	Status string
	Label  string
	Tasks  []TaskRow
}

// TaskRow is one task in the list — route, subject, status, and the path to its
// detail page (reverse-routed, never string-built; rule no-url-string-building).
type TaskRow struct {
	ID       int64
	Route    string
	Subject  string
	Status   string
	ShowPath string
}

// statusOrder is the display order of the status groups (docs/08): the buckets
// that most want attention first. Each carries a human label.
var statusOrder = []struct {
	Status tasks.Status
	Label  string
}{
	{tasks.AwaitingAgent, "Awaiting agent"},
	{tasks.AwaitingUser, "Awaiting user"},
	{tasks.Open, "Open"},
	{tasks.Done, "Done"},
}

// groupTasks buckets tasks by status in statusOrder, newest-first within each
// (model order is creation order, so newest is the reverse). rows builds each
// TaskRow, supplying the reverse-routed detail path. Empty groups are omitted.
func groupTasks(all []tasks.Task, showPath func(id int64) string) []TaskGroup {
	byStatus := map[tasks.Status][]TaskRow{}
	// Walk newest-first so each bucket ends up newest-first.
	for i := len(all) - 1; i >= 0; i-- {
		t := all[i]
		byStatus[t.Status] = append(byStatus[t.Status], TaskRow{
			ID:       int64(t.ID),
			Route:    t.Route,
			Subject:  t.Subject,
			Status:   string(t.Status),
			ShowPath: showPath(int64(t.ID)),
		})
	}
	var groups []TaskGroup
	for _, s := range statusOrder {
		rows := byStatus[s.Status]
		if len(rows) == 0 {
			continue
		}
		groups = append(groups, TaskGroup{Status: string(s.Status), Label: s.Label, Tasks: rows})
	}
	return groups
}

// RenderTasks writes the full task-list page (docs/08). Pages render with
// RenderPage so the full-<html> document composes with its sub-components.
func RenderTasks(ctx context.Context, w io.Writer, engine *htmlc.Engine, view TasksView) error {
	return engine.RenderPage(ctx, w, "view_tasks", map[string]any{
		"tasks": view,
	})
}
