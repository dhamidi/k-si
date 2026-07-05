package agents

import (
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/workspace"
)

// Edges is everything agents touches in the world. Real implementations are
// wired in cmd/kasi/main.go; simulated twins live in this package (docs/12).
type Edges struct {
	Clock   runtime.Clock
	Harness Harness
	Work    workspace.Workspace
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

// SimEdges is the full simulated set — what `kasi test` assembles by default,
// and the simulated twin the twin rule demands (docs/12). The harness and
// workspace are wired to a fresh, private sim pair; in a real scenario run the
// runner injects a SHARED workspace + harness across modules (so out/ and the
// transcript written by the harness are visible to tasks), so this fresh pair is
// only used by the replay/cassette paths that never drive the edges.
func SimEdges() Edges {
	work := workspace.NewMemory()
	return Edges{
		Clock:   runtime.SimClock(),
		Harness: NewSimHarness(work),
		Work:    work,
	}
}
