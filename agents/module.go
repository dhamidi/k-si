package agents

import (
	"github.com/dhamidi/k-si/datastore"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/secrets"
	"github.com/dhamidi/k-si/store"
	"github.com/dhamidi/k-si/workspace"
)

// Edges is everything agents touches in the world. Real implementations are
// wired in cmd/kasi/main.go; simulated twins live in this package (docs/12).
type Edges struct {
	// the agent's persistent data store, symlinked into every run (Flow F)
	Store datastore.Store
	Clock runtime.Clock
	// Harnesses is the registry of agent harnesses keyed by name (decision-024),
	// built at boot in cmd/kasi. A run is pinned to one name and every edge call
	// resolves through this map by that name (resolveHarness), so a restart drives
	// the SAME harness that launched. The sim/recorded/recording twins register
	// under EVERY selectable name, so a scenario pinning any harness runs the twin.
	Harnesses map[string]Harness
	Work      workspace.Workspace
	// Secrets resolves secret:// references into the run environment at the edge
	// (Flow C, decision-004) — plaintext never enters the model or the log.
	Secrets secrets.Secrets
	// Content is read at run start to provision every learned skill into the
	// workspace skills/ box (Flow D, decision-009).
	Content store.Content
	// ControlURL is the loopback origin (e.g. http://127.0.0.1:8787) the agent
	// POSTs notifications to; injected into the run env as KASI_CONTROL_URL
	// (feature-notifications.md). A plain config string, not an interface edge —
	// mirrors email.Edges.BaseURL.
	ControlURL string
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
	registerRecordNotifyToken(mod)
	registerRecordSession(mod)
	registerLaunchAgentRun(mod)
	registerSetMaxConcurrentRuns(mod)
	registerSetWorkerHarness(mod)
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
		Store:     datastore.NewSim(),
		Clock:     runtime.SimClock(),
		Harnesses: OverEveryName(NewSimHarness(work)),
		Work:      work,
		Secrets:   secrets.NewSim(),
		Content:   store.NewMemoryContent(),
		// A harmless placeholder; sim runs don't POST through it.
		ControlURL: "http://127.0.0.1:0",
	}
}
