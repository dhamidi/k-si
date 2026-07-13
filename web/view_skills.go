package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"

	"github.com/dhamidi/k-si/skills"
)

// SkillsView is the data view_skills.vue renders — the skills registry (docs/08,
// Flow D decision-009/010): every authored skill, newest-first, each a link to
// its detail page. Built from skills.All by the route handler, never a raw model
// object (docs/08, docs/15). Single column, mobile-first, structure before style.
type SkillsView struct {
	Skills []SkillRow
	// Nav is the shared top-level navbar site_nav.vue renders (navView) — the same
	// five entries on every page, this one lit.
	Nav NavView
}

// SkillRow is one skill in the list — its name, description, origin
// (agent/ui), and the reverse-routed path to its detail page (never
// string-built; rule no-url-string-building).
type SkillRow struct {
	Name        string
	Description string
	Origin      string
	ShowPath    string
	// DeletePath is the reverse-routed POST target that removes this skill (Flow D
	// Ask 2). The row's inline Remove control submits here; a skill lives in two
	// homes (the tar blob and the registry) and the handler clears both.
	DeletePath string
}

// skillRows builds the list rows from the registry, newest-first (model order is
// creation order, so newest is the reverse), each carrying its reverse-routed
// detail and delete paths.
func skillRows(all []skills.Skill, showPath, deletePath func(name string) string) []SkillRow {
	rows := make([]SkillRow, 0, len(all))
	for i := len(all) - 1; i >= 0; i-- {
		sk := all[i]
		rows = append(rows, SkillRow{
			Name:        sk.Name,
			Description: sk.Description,
			Origin:      sk.Origin,
			ShowPath:    showPath(sk.Name),
			DeletePath:  deletePath(sk.Name),
		})
	}
	return rows
}

// RenderSkills writes the full skills-list page (docs/08). Pages render with
// RenderPage so the full-<html> document composes with its sub-components.
func RenderSkills(ctx context.Context, w io.Writer, engine *htmlc.Engine, view SkillsView) error {
	return engine.RenderPage(ctx, w, "view_skills", map[string]any{
		"skills": view,
	})
}
