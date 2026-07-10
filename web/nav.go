package web

// NavItem is one top-level nav entry rendered by site_nav.vue: its label, its
// reverse-routed path, and whether it is the section currently being viewed. The
// Active boolean is decided in Go (navView), never by comparing strings in the
// template — htmlc's expr compares a defined string type unequal to a literal, so
// the boolean crosses the boundary already resolved (docs/08).
type NavItem struct {
	Label  string
	Path   string
	Active bool
}

// NavView is the shared top-level navigation model every page carries as its Nav
// field (docs/08): the same entries in the same order, so the nav is
// identical across pages, with Active set on the current one. Built by
// (*Server).navView, never string-built (rule no-url-string-building).
type NavView struct {
	Items []NavItem
}

// navTopLevel is the ordered list of the top-level sections and their route
// names — the single source of truth for what the navbar shows and in what order.
// The counter canary at "/" is deliberately absent: it is not a section.
var navTopLevel = []struct {
	Label string
	Route string
}{
	{"Tasks", "tasks.index"},
	{"Skills", "skills.index"},
	{"Memory", "memory.index"},
	{"Apps", "apps.index"},
	{"Store", "store.index"},
	{"Settings", "settings.index"},
}

// navView builds the shared top-level nav, reverse-routing all entries and
// marking the one whose route name matches active (rule no-url-string-building).
// active is the current page's route name (e.g. "tasks.index"); a sub-page passes
// its section's index route (a task or transcript passes "tasks.index", a setting
// "settings.index", a skill or skill-file "skills.index") so the whole section
// stays lit.
func (s *Server) navView(active string) NavView {
	items := make([]NavItem, 0, len(navTopLevel))
	for _, e := range navTopLevel {
		path, _ := s.router.Path(e.Route, nil)
		items = append(items, NavItem{Label: e.Label, Path: path, Active: e.Route == active})
	}
	return NavView{Items: items}
}
