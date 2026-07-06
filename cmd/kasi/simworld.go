package main

// The simulated external world a scenario runs against: the content tables, the
// mail provider, the workspace tree, and the harness — each a domain's in-memory
// twin (docs/12), SHARED across the modules that touch it (email and tasks both
// see the same outbox rows; the harness writes the out/ files tasks harvests).
//
// The runner keeps one simWorld per instance across `crash`/`restart`: a crash
// discards the App (model + goroutines) but not the world, exactly as a real
// process keeps its disk and databases (docs/13). `use` starts a fresh one.

import (
	"github.com/dhamidi/k-si/agents"
	"github.com/dhamidi/k-si/counter"
	"github.com/dhamidi/k-si/email"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/store"
	"github.com/dhamidi/k-si/tasks"
	"github.com/dhamidi/k-si/workspace"
)

type simWorld struct {
	content *store.MemoryContent
	mail    *email.SimMail
	work    *workspace.Memory
	harness *agents.SimHarness

	// inboundSeq mints deterministic Message-IDs for delivered mail, so a
	// scenario's threading keys are stable across runs and replays (docs/13).
	inboundSeq int
}

func newSimWorld() *simWorld {
	content := store.NewMemoryContent()
	work := workspace.NewMemory()
	return &simWorld{
		content: content,
		mail:    email.NewSimMail(content),
		work:    work,
		harness: agents.NewSimHarness(work),
	}
}

// crash resets the world's EPHEMERAL edge state — what a killed process loses.
// The content store, the sent mailbox, and the workspace tree survive (they are
// disk and the outside world); the harness's live-run registry does not: its
// "processes" die, so a fresh harness takes their place, and any still-draining
// agent-watch goroutine from the old App operates on the discarded harness, never
// racing the resumed run on shared rendezvous channels (docs/05, docs/13).
func (w *simWorld) crash() {
	w.harness = agents.NewSimHarness(w.work)
}

// assembleSim wires the one module list to the shared sim world and clock — the
// simulation-ring counterpart of main.go's real-edge assembly (docs/12). Only
// the scenario boot uses this; refold and cassette replay use assembly(true),
// whose isolated SimEdges never drive an effect.
func assembleSim(w *simWorld, clock runtime.Clock) []*runtime.Module {
	return []*runtime.Module{
		counter.Module(counter.Edges{Clock: clock}),
		email.Module(email.Edges{Clock: clock, Mail: w.mail, Content: w.content, Work: w.work, BaseURL: "https://kasi.test"}),
		tasks.Module(tasks.Edges{Clock: clock, Work: w.work, Content: w.content}),
		agents.Module(agents.Edges{Clock: clock, Harness: w.harness, Work: w.work}),
	}
}
