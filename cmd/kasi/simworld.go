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
	"github.com/dhamidi/k-si/cassette"
	"github.com/dhamidi/k-si/counter"
	"github.com/dhamidi/k-si/email"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/store"
	"github.com/dhamidi/k-si/tasks"
	"github.com/dhamidi/k-si/workspace"
)

// simWorld is the external world a scenario runs against. content and mail are
// the sim twins in EVERY ring — the deterministic pipeline the staleness check
// depends on (docs/13) — while work and harness vary: sim runs the scripted
// SimHarness against a memory tree, recorded self-plays a committed cassette,
// and live drives the real Claude harness (wrapped in the recording decorator)
// against a real OS tree, capturing a cassette as it goes.
type simWorld struct {
	ring    string
	content *store.MemoryContent
	mail    *email.SimMail
	work    workspace.Workspace // the workspace wired into Edges (memory or OS)
	harness agents.Harness      // the harness wired into Edges

	sim       *agents.SimHarness       // set for ring "sim" (nil otherwise)
	recorded  *agents.RecordedHarness  // set for ring "recorded"
	recording *agents.RecordingHarness // set for ring "live"
	cassette  cassette.HarnessCassette // the loaded cassette (ring "recorded")
	workdir   string                   // the live OS workspace dir (ring "live"), for cleanup

	// inboundSeq mints deterministic Message-IDs for delivered mail, so a
	// scenario's threading keys are stable across runs and replays (docs/13).
	inboundSeq int
}

func newSimWorld() *simWorld {
	content := store.NewMemoryContent()
	work := workspace.NewMemory()
	sim := agents.NewSimHarness(work)
	return &simWorld{
		ring:    "sim",
		content: content,
		mail:    email.NewSimMail(content),
		work:    work,
		harness: sim,
		sim:     sim,
	}
}

// newRecordedWorld builds the recorded ring's world: the same sim content/mail
// twins, a memory workspace, and a RecordedHarness that self-plays the committed
// cassette when the `agent` command triggers it (docs/13).
func newRecordedWorld(c cassette.HarnessCassette) *simWorld {
	content := store.NewMemoryContent()
	work := workspace.NewMemory()
	recorded := agents.NewRecordedHarness(work, c)
	return &simWorld{
		ring:     "recorded",
		content:  content,
		mail:     email.NewSimMail(content),
		work:     work,
		harness:  recorded,
		recorded: recorded,
		cassette: c,
	}
}

// newLiveWorld builds the live-capture world: the SAME deterministic sim
// content/mail twins as sim/recorded (so the captured in/ bytes match what
// replay lays down), but a real OS workspace and the real Claude harness wrapped
// in the recording decorator (docs/13). Only work and harness are real.
func newLiveWorld(workdir string) *simWorld {
	content := store.NewMemoryContent()
	work := workspace.NewOS(workdir)
	recording := agents.NewRecordingHarness(agents.NewClaude(workdir), work)
	return &simWorld{
		ring:      "live",
		content:   content,
		mail:      email.NewSimMail(content),
		work:      work,
		harness:   recording,
		recording: recording,
		workdir:   workdir,
	}
}

// crash resets the world's EPHEMERAL edge state — what a killed process loses.
// The content store, the sent mailbox, and the workspace tree survive (they are
// disk and the outside world); the harness's live-run registry does not: its
// "processes" die, so a fresh harness takes their place, and any still-draining
// agent-watch goroutine from the old App operates on the discarded harness, never
// racing the resumed run on shared rendezvous channels (docs/05, docs/13).
func (w *simWorld) crash() {
	if w.ring == "sim" {
		w.sim = agents.NewSimHarness(w.work)
		w.harness = w.sim
		return
	}
	// crash + recorded/live isn't a scenario yet — leave the world untouched.
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
