package skills

import "github.com/dhamidi/k-si/runtime"

// Model is the skills slice of the application model (docs/15): the content-free
// registry of skills agents have authored (Flow D, decision-010). A skill's tree
// lives in the store's `skill` table; the model holds only light metadata, so
// replay stays cheap. A SLICE, never a map, for deterministic replay convergence.
type Model struct {
	Skills []Skill `json:"skills"`
}

// Skill is one registry entry — light metadata only, NO content (the tar tree is
// in the store, keyed by ID). name is UNIQUE; a re-authored skill updates the
// entry in place and bumps Version (decision-010).
type Skill struct {
	ID          int64  `json:"id"`
	OriginTask  int64  `json:"origin_task"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Origin      string `json:"origin"` // 'ui' | 'agent'
	Version     int    `json:"version"`
}

// slice reads the skills Model out of a whole-model View — the typed accessor
// every exported read helper funnels through (docs/15).
func slice(v runtime.View) Model {
	return runtime.Slice[Model](v, "skills")
}

// All returns a copy of the registry in model order — the list read the browse
// UI groups and sorts (docs/08). The copy keeps callers from aliasing
// model-owned memory.
func All(v runtime.View) []Skill {
	m := slice(v)
	out := make([]Skill, len(m.Skills))
	copy(out, m.Skills)
	return out
}

// Get returns the skill with id, if present — the exported point read.
func Get(v runtime.View, id int64) (Skill, bool) {
	m := slice(v)
	if i := m.find(id); i >= 0 {
		return m.Skills[i], true
	}
	return Skill{}, false
}

// find returns the index of the skill with id in the slice, or -1.
func (m Model) find(id int64) int {
	for i := range m.Skills {
		if m.Skills[i].ID == id {
			return i
		}
	}
	return -1
}

// findName returns the index of the skill with the given name, or -1. A skill's
// name is UNIQUE, so a re-authored skill replaces its entry (decision-010).
func (m Model) findName(name string) int {
	for i := range m.Skills {
		if m.Skills[i].Name == name {
			return i
		}
	}
	return -1
}
