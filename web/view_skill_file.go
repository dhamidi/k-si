package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"
)

// SkillFileView is the data view_skill_file.vue renders — one entry of a skill's
// tree (docs/08, Flow D decision-010): its path and raw text, read from the tar
// via skilltree.Read. This is what makes the tree browsable. Built by the route
// handler, never a raw model object (docs/08, docs/15).
type SkillFileView struct {
	SkillName string
	Path      string
	Content   string
	// BackPath links back to the skill's detail page (reverse-routed) — the local
	// secondary crumb, kept beside the shared top-level nav.
	BackPath string
	// Nav is the shared top-level navbar (navView) — a skill file lights the Skills
	// section.
	Nav NavView
}

// RenderSkillFile writes the full skill-file page (docs/08).
func RenderSkillFile(ctx context.Context, w io.Writer, engine *htmlc.Engine, view SkillFileView) error {
	return engine.RenderPage(ctx, w, "view_skill_file", map[string]any{
		"file": view,
	})
}
