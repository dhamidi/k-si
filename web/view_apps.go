package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"
)

// AppsView is the data view_apps.vue renders — the /apps page (docs/08,
// feature-apps.md): every registered app, its URL, käsi's registry status, and
// its live state on the machine. Built by the route handler from apps.All plus
// a live Runner read, never a raw model object (docs/08, docs/15). Host-gated,
// no token (decision-006) — the same trust model as the other browse pages. A
// view never writes (docs/08): registering and removing an app is `kasi app`'s
// job, not this page's.
type AppsView struct {
	Apps []AppRow
	// TasksPath is the cross-nav back to the hub list page — the pattern skills
	// and memory already carry. Reverse-routed, never string-built (rule
	// no-url-string-building).
	TasksPath string
}

// AppRow is one app in the list. Name, URL, and Status come from the registry
// (käsi's own log-derived state: registered, running, or removing); Live and
// Logs are read from the machine at render time — "up"/"down"/"unknown" and
// its most recent journald lines (feature-apps.md).
type AppRow struct {
	Name   string
	URL    string
	Status string
	Live   string
	Logs   []string
}

// RenderApps writes the full page (docs/08).
func RenderApps(ctx context.Context, w io.Writer, engine *htmlc.Engine, view AppsView) error {
	return engine.RenderPage(ctx, w, "view_apps", map[string]any{
		"apps": view,
	})
}
