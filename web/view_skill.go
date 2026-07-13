package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"
)

// SkillView is the data view_skill.vue renders — one skill's detail (docs/08,
// Flow D decision-009/010): its metadata (name, description, origin,
// origin-task, version), its file tree (each entry linking to the file view),
// and the SKILL.md body inline. Built from the content store's SkillByName (the
// tar tree) by the route handler, never a raw model object (docs/08, docs/15).
type SkillView struct {
	Name        string
	Description string
	Origin      string
	Version     int
	// OriginTask is the task that authored an agent-origin skill; OriginTaskPath
	// is its reverse-routed detail link. Present is false for ui-origin skills or
	// when there is no originating task.
	OriginTask     int64
	OriginTaskPath string
	HasOriginTask  bool
	// Files are the tree's entry paths, each with a link to its file view.
	Files []SkillFileLink
	// SkillMD is the raw SKILL.md body, shown inline in a <pre> (structure first).
	SkillMD string
	// DeletePath is the reverse-routed POST target that removes this skill (Flow D
	// Ask 2). The detail header's Remove control submits here.
	DeletePath string
	// Nav is the shared top-level navbar (navView) — a skill detail lights the
	// Skills section.
	Nav NavView
}

// SkillFileLink is one tree entry in the detail's file list: its relative path
// and the reverse-routed link to its file view (never string-built).
type SkillFileLink struct {
	Path     string
	FilePath string
}

// RenderSkill writes the full skill-detail page (docs/08).
func RenderSkill(ctx context.Context, w io.Writer, engine *htmlc.Engine, view SkillView) error {
	return engine.RenderPage(ctx, w, "view_skill", map[string]any{
		"skill": view,
	})
}
