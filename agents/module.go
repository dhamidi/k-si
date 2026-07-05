package agents

import "github.com/dhamidi/k-si/runtime"

// Edges is everything agents touches in the world. Real implementations are
// wired in cmd/kasi/main.go; simulated twins live in this package (docs/12).
type Edges struct {
	Clock runtime.Clock
}

// Module bundles harness invocation, agent runs, and transcripts (docs/01).
func Module(e Edges) *runtime.Module {
	mod := runtime.NewModule("agents", Model{}, e)

	registerSpawnAgentRun(mod)
	registerStopAgentRun(mod)
	registerFinishAgentRun(mod)
	registerStartAgentRun(mod)
	registerSignalAgentRun(mod)
	runtime.Subscribe(mod, agentWatchSubs)
	return mod
}

// SimEdges is the full simulated set — what `kasi test` assembles by
// default, and the simulated twin the twin rule demands (docs/12).
func SimEdges() Edges {
	return Edges{
		Clock: runtime.SimClock(),
	}
}
