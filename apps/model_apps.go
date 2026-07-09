package apps

import "github.com/dhamidi/k-si/runtime"

// Model is the apps slice of the application model (docs/15): the registry
// of apps käsi keeps running, rebuilt by replaying register-app directives
// (feature-apps.md). A SLICE, never a map, for deterministic replay
// convergence.
type Model struct {
	Apps []App `json:"apps"`
}

// slice reads the apps Model out of a whole-model View — the typed accessor
// every exported read helper funnels through (docs/15).
func slice(v runtime.View) Model {
	return runtime.Slice[Model](v, "apps")
}

// All returns a copy of the registry in model order — the list the /apps page
// renders (feature-apps.md). The copy keeps callers from aliasing model-owned
// apps.
func All(v runtime.View) []App {
	m := slice(v)
	out := make([]App, len(m.Apps))
	copy(out, m.Apps)
	return out
}

// PortMin and PortMax bound the forwarded range apps are assigned from
// (feature-apps.md). FreePort hands out the lowest unused port in this band, so
// the assignment is deterministic — the same registry always yields the same
// next port, which replay-convergence depends on (docs/15).
const (
	PortMin = 3000
	PortMax = 9999
)

// FreePort returns the lowest port in [PortMin,PortMax] not already claimed by a
// registered app, or 0 when the band is full. Deterministic — it scans the
// model, never a clock or random source — so the control endpoint's port choice
// is replayable (docs/15).
func FreePort(v runtime.View) int {
	m := slice(v)
	used := make(map[int]bool, len(m.Apps))
	for _, a := range m.Apps {
		used[a.Port] = true
	}
	for p := PortMin; p <= PortMax; p++ {
		if !used[p] {
			return p
		}
	}
	return 0
}

// Find returns the registered app with the given name and true, or a zero App
// and false. The control endpoint uses it to reuse an existing app's port on a
// re-add rather than allocating a new one (feature-apps.md: a name is unique).
func Find(v runtime.View, name string) (App, bool) {
	m := slice(v)
	if i := m.findName(name); i >= 0 {
		return m.Apps[i], true
	}
	return App{}, false
}

// Running returns a copy of the apps whose unit is up — the set provisioned
// into a run's in/apps.json so the agent can call them on localhost while it
// works a task (feature-apps.md, "the agent uses apps"). Only a running app is
// callable, so a merely-registered or removing app is left out.
func Running(v runtime.View) []App {
	m := slice(v)
	var out []App
	for i := range m.Apps {
		if m.Apps[i].Status == StatusRunning {
			out = append(out, m.Apps[i])
		}
	}
	return out
}

// findName returns the index of the app with the given name, or -1. A name
// is UNIQUE (feature-apps.md): re-registering an existing name replaces that
// app's entry in place, the way remembering an existing memory updates it.
func (m Model) findName(name string) int {
	for i := range m.Apps {
		if m.Apps[i].Name == name {
			return i
		}
	}
	return -1
}
