package memory

import "github.com/dhamidi/k-si/runtime"

// Model is the memory slice of the application model (docs/15): the collection
// of durable facts käsi remembers, rebuilt by replaying remember/forget
// directives (feature-memory.md). A SLICE, never a map, for deterministic replay
// convergence.
type Model struct {
	Memories []Memory `json:"memories"`
}

// Memory is one remembered fact. Name is its identity (unique; the file's slug).
// Content is the RAW memory file — frontmatter and body — the fact as it lives in
// the log. Description is DERIVED from Content's frontmatter by the reducer on
// every replay (store the raw fact in the log, derive everything else on replay),
// so a parser change re-derives it with no migration.
type Memory struct {
	Name        string `json:"name"`
	Content     []byte `json:"content"`
	Description string `json:"description"`
}

// slice reads the memory Model out of a whole-model View — the typed accessor
// every exported read helper funnels through (docs/15).
func slice(v runtime.View) Model {
	return runtime.Slice[Model](v, "memory")
}

// All returns a copy of the collection in model order — the list the browse UI
// renders and provisioning lays into every run (feature-memory.md). The copy
// keeps callers from aliasing model-owned memory.
func All(v runtime.View) []Memory {
	m := slice(v)
	out := make([]Memory, len(m.Memories))
	copy(out, m.Memories)
	return out
}

// findName returns the index of the memory with the given name, or -1. A name is
// UNIQUE: a re-remembered fact replaces its entry (feature-memory.md upsert).
func (m Model) findName(name string) int {
	for i := range m.Memories {
		if m.Memories[i].Name == name {
			return i
		}
	}
	return -1
}
