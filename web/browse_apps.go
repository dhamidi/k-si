package web

import (
	"context"
	"log"
	"net/http"

	"github.com/dhamidi/k-si/apps"
)

// AppRunner reads an app's live state from the machine at render time: whether
// its systemd unit is up, and its recent journald lines (feature-apps.md). This
// is the narrow, read-only slice of apprunner.Runner the /apps page needs — the
// web edge never writes through it; registering and removing an app is `kasi
// app`'s job via the control endpoint, not this page's (docs/08: a view never
// writes).
type AppRunner interface {
	Status(ctx context.Context, name string) (up bool, err error)
	Logs(ctx context.Context, name string, n int) ([]string, error)
}

// recentAppLogLines is how many journald lines the /apps page shows per app.
const recentAppLogLines = 20

// showApps renders the /apps page (docs/08, feature-apps.md): every registered
// app with its URL, registry status, and live state. Host-gated, no token
// (decision-006). The registry comes from apps.All (the log, replayed); the
// liveness is a live Runner read, degrading gracefully when the edge is absent
// or the machine can't answer — the registry still renders regardless.
func (s *Server) showApps(w http.ResponseWriter, r *http.Request) {
	view := AppsView{
		Apps: s.appRows(r.Context(), apps.All(s.app.View())),
		Nav:  s.navView("apps.index"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := RenderApps(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render apps: %v", err)
	}
}

// appRows builds one row per registered app, each carrying a live Status/Logs
// read from the Runner edge (feature-apps.md: the registry is käsi's, the
// liveness is the machine's).
func (s *Server) appRows(ctx context.Context, all []apps.App) []AppRow {
	rows := make([]AppRow, 0, len(all))
	for _, app := range all {
		live, logs := s.appLiveState(ctx, app.Name)
		rows = append(rows, AppRow{
			Name:   app.Name,
			URL:    app.URL,
			Status: app.Status,
			Live:   live,
			Logs:   logs,
		})
	}
	return rows
}

// appLiveState reads one app's liveness and recent logs from the Runner edge.
// No edge wired, or an error reaching the machine, both degrade to "unknown"
// with no logs — a browse page never fails on a machine it can't currently
// reach.
func (s *Server) appLiveState(ctx context.Context, name string) (string, []string) {
	if s.runner == nil {
		return "unknown", nil
	}

	up, err := s.runner.Status(ctx, name)
	if err != nil {
		log.Printf("web: app %s: status: %v", name, err)
		return "unknown", nil
	}
	live := "down"
	if up {
		live = "up"
	}

	logs, err := s.runner.Logs(ctx, name, recentAppLogLines)
	if err != nil {
		log.Printf("web: app %s: logs: %v", name, err)
		logs = nil
	}
	return live, logs
}
