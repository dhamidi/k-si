package agents

import "github.com/dhamidi/k-si/runtime"

// Model is the agents slice of the application model (docs/15). Runs is an
// ordered slice (not a map) so the model marshals deterministically for the
// replay-convergence standing check (docs/13).
type Model struct {
	Runs []AgentRun `json:"runs"`
}

// slice reads the agents model out of a View for the exported read helpers.
func slice(v runtime.View) Model {
	return runtime.Slice[Model](v, "agents")
}

// RunningRuns returns a copy of every run currently in status "running" — the
// set the agent-watch subscription turns into live watchers (docs/05). Values,
// never pointers into the slice.
func RunningRuns(v runtime.View) []AgentRun {
	s := slice(v)
	var out []AgentRun
	for _, r := range s.Runs {
		if r.Status == StatusRunning {
			out = append(out, r)
		}
	}
	return out
}

// Run returns a copy of the run with the given id, if it exists — the exported
// pure read other domains use to look at a single run (docs/15).
func Run(v runtime.View, runID AgentRunID) (AgentRun, bool) {
	s := slice(v)
	for _, r := range s.Runs {
		if r.ID == runID {
			return r, true
		}
	}
	return AgentRun{}, false
}

// findRun returns the index of the run with runID within a mutable Model slice,
// or -1. Handlers use it to update a run in place (docs/15).
func (m Model) findRun(runID AgentRunID) int {
	for i := range m.Runs {
		if m.Runs[i].ID == runID {
			return i
		}
	}
	return -1
}
