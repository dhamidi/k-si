package tasks

import "github.com/dhamidi/k-si/runtime"

// Model is the tasks slice of the application model (docs/15). It is a SLICE of
// tasks, never a map — a slice marshals in a deterministic order so refolding
// the log converges byte-for-byte on the live model (BRIEF replay-convergence).
type Model struct {
	Tasks []Task `json:"tasks"`
}

// slice reads the tasks Model out of a whole-model View — the typed accessor
// every exported read helper funnels through (docs/15).
func slice(v runtime.View) Model {
	return runtime.Slice[Model](v, "tasks")
}

// find returns the index of the task with id in the slice, or -1.
func (m Model) find(id TaskID) int {
	for i := range m.Tasks {
		if m.Tasks[i].ID == id {
			return i
		}
	}
	return -1
}
