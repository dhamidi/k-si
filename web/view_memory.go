package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"

	"github.com/dhamidi/k-si/memory"
)

// MemoryView is the data view_memory.vue renders (docs/08, feature-memory.md):
// the owner's curation window onto the memory collection — every fact with its
// derived description, an add/edit form, and a per-row forget action. Built from
// memory.All by the route handler, never a raw model object (docs/08, docs/15).
// Single column, mobile-first, structure before style.
type MemoryView struct {
	Memories []MemoryRow
	// Form is the add/edit form object — empty on a plain GET, echoed with values
	// and errors on an invalid submit.
	Form RememberForm
	// SavePath is the POST target of the remember form (add or edit). Reverse-routed,
	// never string-built (rule no-url-string-building).
	SavePath string
	// Nav is the shared top-level navbar site_nav.vue renders (navView) — the same
	// five entries on every page, this one lit.
	Nav NavView
}

// MemoryRow is one memory in the list — its name, derived description, the raw
// Content (for the edit textarea), and the reverse-routed forget POST target.
type MemoryRow struct {
	Name        string
	Description string
	Content     string
	ForgetPath  string
}

// memoryRows builds the list rows from the collection, newest-first (model order
// is creation order, so newest is the reverse), each carrying its raw content for
// editing and its reverse-routed forget target.
func memoryRows(all []memory.Memory, forgetPath func(name string) string) []MemoryRow {
	rows := make([]MemoryRow, 0, len(all))
	for i := len(all) - 1; i >= 0; i-- {
		m := all[i]
		rows = append(rows, MemoryRow{
			Name:        m.Name,
			Description: m.Description,
			Content:     string(m.Content),
			ForgetPath:  forgetPath(m.Name),
		})
	}
	return rows
}

// RenderMemory writes the full memory page (docs/08). Pages render with
// RenderPage so the full-<html> document composes with its sub-components.
func RenderMemory(ctx context.Context, w io.Writer, engine *htmlc.Engine, view MemoryView) error {
	return engine.RenderPage(ctx, w, "view_memory", map[string]any{
		"memory": view,
	})
}
