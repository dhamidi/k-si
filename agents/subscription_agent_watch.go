package agents

import "github.com/dhamidi/k-si/runtime"

// agent-watch — one watcher per running run; emit finish-agent-run when the harness exits
//
// A pure function from state to the set of sources that should be running,
// each with a stable id; the runtime diffs and starts/stops them (docs/01).
func agentWatchSubs(v runtime.View, s Model) []runtime.Sub {
	return nil
}
