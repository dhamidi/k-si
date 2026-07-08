package agents

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/workspace"
)

// SimHarness is the in-memory twin of the Claude harness (docs/05, docs/12): it
// stands in for a worker process, delivering a scripted turn on demand and
// rendezvousing with the agent-watch subscription so a stimulus settles the
// WHOLE turn (spawn → run → finish-agent-run enqueued) before returning.
//
// Concurrency model:
//
//   - The live-run registry (runs) is guarded by mu and is EPHEMERAL — a fresh
//     SimHarness after a simulated crash has none. Only the shared workspace
//     (out/, transcripts) survives a crash; the registry is rebuilt as the
//     agent-watch source (the sole launcher, decision-015) drives start-agent-run
//     for each orphaned run, which re-registers it via Start/Resume.
//   - A run is keyed by taskID; its runID is tracked so a new turn's Start/Wait
//     replaces the previous turn's stale entry, while Start and Wait for the
//     SAME turn converge on one entry regardless of which runs first.
//   - turn (unbuffered) carries the delivered turn from DeliverTurn to the
//     blocked Wait; emitted is closed by MarkEmitted (called by the subscription
//     right after it emits finish-agent-run) to release DeliverTurn; cancel is
//     closed by Signal to make a blocked Wait return Stopped.
type SimHarness struct {
	mu   sync.Mutex
	cond *sync.Cond // broadcast when a run is registered, so DeliverTurn can wait
	work workspace.Workspace
	runs map[int64]*liveRun // keyed by taskID (ephemeral)
	// envs records the resolved run environment last handed to Start/Resume per
	// task, so a scenario can assert a Flow C secret reached the agent edge
	// (decision-004). It is test observability only.
	envs map[int64]map[string]string
}

// liveRun is one registered, in-flight turn.
type liveRun struct {
	runID   int64
	session string
	turn    chan turnData
	emitted chan struct{}
	cancel  chan struct{}

	emitOnce   sync.Once
	cancelOnce sync.Once
}

// turnData is a scripted turn handed from DeliverTurn to Wait.
type turnData struct {
	exit           int
	outManifest    []string
	transcriptPath string
}

var _ Harness = (*SimHarness)(nil)

// NewSimHarness builds the sim harness over a shared workspace (the runner wires
// the same workspace into the tasks module so harvested out/ and captured
// transcripts line up).
func NewSimHarness(work workspace.Workspace) *SimHarness {
	h := &SimHarness{
		work: work,
		runs: make(map[int64]*liveRun),
		envs: make(map[int64]map[string]string),
	}
	h.cond = sync.NewCond(&h.mu)
	return h
}

// Start registers a live run for a task's first turn and returns immediately;
// the instance then sits quiescent with the watch subscription blocked in Wait.
func (h *SimHarness) Start(ctx context.Context, taskID, runID int64, env map[string]string) (Handle, error) {
	session := sessionFor(taskID)
	h.recordEnv(taskID, env)
	h.register(taskID, runID, session)
	return Handle{TaskID: taskID, RunID: runID, Session: session}, nil
}

// Resume registers a live run continuing an existing session for a later turn.
func (h *SimHarness) Resume(ctx context.Context, taskID, runID int64, session string, env map[string]string) (Handle, error) {
	if session == "" {
		session = sessionFor(taskID)
	}
	h.recordEnv(taskID, env)
	h.register(taskID, runID, session)
	return Handle{TaskID: taskID, RunID: runID, Session: session}, nil
}

// recordEnv stores the run environment for test assertions (EnvFor).
func (h *SimHarness) recordEnv(taskID int64, env map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.envs[taskID] = env
}

// EnvFor returns the environment last handed to Start/Resume for a task — the
// resolved Flow C secrets, for a scenario to assert delivery (decision-004).
func (h *SimHarness) EnvFor(taskID int64) map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.envs[taskID]
}

// Wait blocks until DeliverTurn hands this run its turn, or ctx is cancelled
// (stop signalled, or a crash cancels the sub), or Signal closes the run. It
// first blocks until the source-driven start-agent-run registers the run
// (awaitRegistered), mirroring the real harness's awaitRun — the agent-watch
// source is the sole launcher (decision-015). On a ctx cancel before the run
// registers it returns Stopped.
func (h *SimHarness) Wait(ctx context.Context, hd Handle) Result {
	lr := h.awaitRegistered(ctx, hd)
	tp := transcriptPath(hd.TaskID, hd.RunID)
	if lr == nil {
		return Result{Stopped: true, TranscriptPath: tp}
	}
	select {
	case <-ctx.Done():
		return Result{Stopped: true, TranscriptPath: tp}
	case <-lr.cancel:
		return Result{Stopped: true, TranscriptPath: tp}
	case td := <-lr.turn:
		return Result{
			Exit:           td.exit,
			TranscriptPath: td.transcriptPath,
			OutManifest:    td.outManifest,
			Stopped:        false,
		}
	}
}

// awaitRegistered blocks until Start/Resume registers this exact run, or ctx
// cancels. The agent-watch source drives start-agent-run to register it (the sole
// launcher, decision-015); before this change Wait auto-registered, which silently
// modelled a resume the real harness never performed (a twin-parity hole).
func (h *SimHarness) awaitRegistered(ctx context.Context, hd Handle) *liveRun {
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			h.mu.Lock()
			h.cond.Broadcast()
			h.mu.Unlock()
		case <-stop:
		}
	}()

	h.mu.Lock()
	defer h.mu.Unlock()
	for {
		if lr := h.runs[hd.TaskID]; lr != nil && lr.runID == hd.RunID {
			return lr
		}
		if ctx.Err() != nil {
			return nil
		}
		h.cond.Wait()
	}
}

// Signal closes the currently-running run for taskID so a blocked Wait returns
// Stopped. Used by the stop path (signal-agent-run effect).
func (h *SimHarness) Signal(ctx context.Context, hd Handle) error {
	h.mu.Lock()
	lr := h.runs[hd.TaskID]
	h.mu.Unlock()
	if lr != nil {
		lr.cancelOnce.Do(func() { close(lr.cancel) })
	}
	return nil
}

// DeliverTurn is the test-vocabulary entry point (`agent { out <file> <content>
// ... del in/<path> ... exit N }`). It writes the turn's out/ files, applies any
// in/ deletions (an agent forgetting a memory, feature-memory.md), and writes a
// transcript into the shared workspace, hands the turn to the blocked Wait for the
// running run of taskID, then BLOCKS until MarkEmitted releases it — guaranteeing
// finish-agent-run is enqueued before it returns, so the stimulus's Settle() waits
// for the whole turn. deletions are in/-box-relative paths ("memory/reply-style.md").
func (h *SimHarness) DeliverTurn(taskID int64, outParts []mime.Part, deletions []string, exit int) error {
	// Wait for the run to register: the agent-watch source drives start-agent-run
	// (the sole launcher, decision-015) which registers the run via Start/Resume in
	// its own goroutine, which can lag the `agent` stimulus. The caller already
	// found a running run in the model, so one will register; blocking here removes
	// that race deterministically.
	h.mu.Lock()
	for h.runs[taskID] == nil {
		h.cond.Wait()
	}
	lr := h.runs[taskID]
	h.mu.Unlock()

	if err := h.work.WriteOut(taskID, outParts); err != nil {
		return fmt.Errorf("agents: DeliverTurn: write out: %w", err)
	}
	for _, rel := range deletions {
		if err := h.work.DeleteIn(taskID, rel); err != nil {
			return fmt.Errorf("agents: DeliverTurn: delete in/%s: %w", rel, err)
		}
	}
	transcript := []byte(fmt.Sprintf(`{"task":%d,"run":%d,"exit":%d}`+"\n", taskID, lr.runID, exit))
	if err := h.work.WriteTranscript(taskID, lr.runID, transcript); err != nil {
		return fmt.Errorf("agents: DeliverTurn: write transcript: %w", err)
	}

	// The manifest is the WHOLE out/ box, not just this turn's writes: the on-disk
	// harness reports what it finds by WALKING the out/ directory, which accumulates
	// across turns unless reset. Reading it back keeps the sim a faithful twin — a
	// stale reply.txt a prior turn left is visible here exactly as the real harness
	// sees it, so the gate can catch a re-send. start-agent-run's ResetOut is what
	// keeps this box to a single turn (decision-019). Reporting only this turn's parts
	// (the prior behaviour) hid the bug: the sim never re-sent a stale reply.
	manifest, err := outBoxManifest(h.work, taskID)
	if err != nil {
		return fmt.Errorf("agents: DeliverTurn: out manifest: %w", err)
	}

	td := turnData{
		exit:           exit,
		outManifest:    manifest,
		transcriptPath: transcriptPath(taskID, lr.runID),
	}
	lr.turn <- td // hand off to the blocked Wait
	<-lr.emitted  // wait until finish-agent-run has been enqueued
	return nil
}

// MarkEmitted is called by the agent-watch subscription body right after it
// emits finish-agent-run, releasing the matching DeliverTurn.
func (h *SimHarness) MarkEmitted(hd Handle) {
	h.mu.Lock()
	lr := h.runs[hd.TaskID]
	h.mu.Unlock()
	if lr != nil && lr.runID == hd.RunID {
		lr.emitOnce.Do(func() { close(lr.emitted) })
	}
}

// register creates (or, for the same turn, reuses) the live run for taskID.
func (h *SimHarness) register(taskID, runID int64, session string) *liveRun {
	h.mu.Lock()
	defer h.mu.Unlock()
	if lr, ok := h.runs[taskID]; ok && lr.runID == runID {
		return lr
	}
	lr := newLiveRun(runID, session)
	h.runs[taskID] = lr
	h.cond.Broadcast() // wake any DeliverTurn waiting for this run to register
	return lr
}

// IsLive reports whether this process has a registered run matching the handle —
// false after a simulated crash wiped the ephemeral runs map, the signal the
// agent-watch source uses to (re)launch exactly once (decision-015).
func (h *SimHarness) IsLive(hd Handle) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	lr := h.runs[hd.TaskID]
	return lr != nil && lr.runID == hd.RunID
}

func newLiveRun(runID int64, session string) *liveRun {
	return &liveRun{
		runID:   runID,
		session: session,
		turn:    make(chan turnData),
		emitted: make(chan struct{}),
		cancel:  make(chan struct{}),
	}
}

// outBoxManifest lists the WHOLE out/ box as paths relative to out/, sorted — the
// sim mirror of the on-disk harness's recursive out/ walk (c.manifest in the Claude
// adapter). Files lists out/ already sorted; this strips the box prefix. Building it
// from the ACCUMULATED box (not just this turn's parts) is the twin-fidelity fix
// behind decision-019: it is what lets the sim reproduce a stale-out/ re-send.
func outBoxManifest(work workspace.Workspace, taskID int64) ([]string, error) {
	files, err := work.Files(taskID)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, f := range files {
		if rel, ok := strings.CutPrefix(f.Filename, "out/"); ok {
			names = append(names, rel)
		}
	}
	return names, nil
}

// sessionFor is the deterministic session id for a task's continuous
// conversation (docs/05: one task ⇔ one session). It is a valid UUID so the real
// Claude adapter can pass it to `--session-id`; the sim harness treats it as an
// opaque string. (Task ids are small log offsets, so the 12-digit field holds.)
func sessionFor(taskID int64) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", taskID)
}

// transcriptPath is the deterministic workspace-relative transcript location for
// a run; ReadTranscript keys by (taskID, runID), so this is informational.
func transcriptPath(taskID, runID int64) string {
	return fmt.Sprintf("task-%d/transcript-%d.jsonl", taskID, runID)
}
