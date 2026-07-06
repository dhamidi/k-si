package tasks

import "github.com/dhamidi/k-si/runtime"

// Model is the tasks slice of the application model (docs/15). It is a SLICE of
// tasks, never a map — a slice marshals in a deterministic order so refolding
// the log converges byte-for-byte on the live model (BRIEF replay-convergence).
type Model struct {
	Tasks []Task `json:"tasks"`
	// Requests are the UI requests agents raised (Flow C), keyed by the raising
	// run's id. A slice, never a map, for deterministic replay (decision-002).
	Requests []UIRequest `json:"requests"`
	// ReplyFrom is the deliverable From address replies are sent as — configured
	// once via set-reply-from (docs/04). Empty falls back to the routeAddr
	// placeholder, which is fine for the sim ring but not for real delivery.
	ReplyFrom string `json:"reply_from"`
}

// slice reads the tasks Model out of a whole-model View — the typed accessor
// every exported read helper funnels through (docs/15).
func slice(v runtime.View) Model {
	return runtime.Slice[Model](v, "tasks")
}

// All returns a copy of the model's tasks in model order — the list read the
// browse UI groups and sorts (docs/08). It mirrors Get: a typed read funnelled
// through the tasks slice, never a raw model reach. The copy keeps the caller
// from aliasing model-owned memory; grouping/newest-first ordering is the
// view's job, not the model's.
func All(v runtime.View) []Task {
	m := slice(v)
	out := make([]Task, len(m.Tasks))
	copy(out, m.Tasks)
	return out
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

// findRequest returns the index of the UI request keyed by runID, or -1.
func (m Model) findRequest(runID int64) int {
	for i := range m.Requests {
		if m.Requests[i].RunID == runID {
			return i
		}
	}
	return -1
}
