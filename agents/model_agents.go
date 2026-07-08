package agents

import "github.com/dhamidi/k-si/runtime"

// Model is the agents slice of the application model (docs/15). Runs is an
// ordered slice (not a map) so the model marshals deterministically for the
// replay-convergence standing check (docs/13).
type Model struct {
	Runs []AgentRun `json:"runs"`
	// MaxConcurrent caps how many runs may hold a live harness process at once — the
	// OOM breaker (SEV1, decision-016). The sole-launcher subscription launches only
	// the lowest-id MaxConcurrent running runs and leaves the rest queued (still
	// StatusRunning, no process) until a slot frees. Configured via
	// set-max-concurrent-runs (serve -max-concurrent-runs). 0 is unlimited — the
	// sim-ring default, so the gate launches exactly as before.
	MaxConcurrent int `json:"max_concurrent"`
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
