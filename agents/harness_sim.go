package agents

import (
	"context"
	"fmt"
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
//     restarted watch subscriptions call Wait and auto-resume.
//   - A run is keyed by taskID; its runID is tracked so a new turn's Start/Wait
//     replaces the previous turn's stale entry, while Start and Wait for the
//     SAME turn converge on one entry regardless of which runs first.
//   - turn (unbuffered) carries the delivered turn from DeliverTurn to the
//     blocked Wait; emitted is closed by MarkEmitted (called by the subscription
//     right after it emits finish-agent-run) to release DeliverTurn; cancel is
//     closed by Signal to make a blocked Wait return Stopped.
type SimHarness struct {
	mu   sync.Mutex
	work workspace.Workspace
	runs map[int64]*liveRun // keyed by taskID (ephemeral)
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
	return &SimHarness{
		work: work,
		runs: make(map[int64]*liveRun),
	}
}

// Start registers a live run for a task's first turn and returns immediately;
// the instance then sits quiescent with the watch subscription blocked in Wait.
func (h *SimHarness) Start(ctx context.Context, taskID, runID int64) (Handle, error) {
	session := sessionFor(taskID)
	h.register(taskID, runID, session)
	return Handle{TaskID: taskID, RunID: runID, Session: session}, nil
}

// Resume registers a live run continuing an existing session for a later turn.
func (h *SimHarness) Resume(ctx context.Context, taskID, runID int64, session string) (Handle, error) {
	if session == "" {
		session = sessionFor(taskID)
	}
	h.register(taskID, runID, session)
	return Handle{TaskID: taskID, RunID: runID, Session: session}, nil
}

// Wait blocks until DeliverTurn hands this run its turn, or ctx is cancelled
// (stop signalled, or a crash cancels the sub), or Signal closes the run. On a
// missing run (a crash killed the ephemeral registry while the model still says
// "running") it AUTO-REGISTERS the handle as a resume before blocking, so the
// restarted watch subscription reconciles the interrupted run.
func (h *SimHarness) Wait(ctx context.Context, hd Handle) Result {
	lr := h.registerHandle(hd)
	tp := transcriptPath(hd.TaskID, hd.RunID)
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
// ... exit N }`). It writes the turn's out/ files and a transcript into the
// shared workspace, hands the turn to the blocked Wait for the running run of
// taskID, then BLOCKS until MarkEmitted releases it — guaranteeing
// finish-agent-run is enqueued before it returns, so the stimulus's Settle()
// waits for the whole turn.
func (h *SimHarness) DeliverTurn(taskID int64, outParts []mime.Part, exit int) error {
	h.mu.Lock()
	lr := h.runs[taskID]
	h.mu.Unlock()
	if lr == nil {
		return fmt.Errorf("agents: DeliverTurn: no live run for task %d", taskID)
	}

	if err := h.work.WriteOut(taskID, outParts); err != nil {
		return fmt.Errorf("agents: DeliverTurn: write out: %w", err)
	}
	transcript := []byte(fmt.Sprintf(`{"task":%d,"run":%d,"exit":%d}`+"\n", taskID, lr.runID, exit))
	if err := h.work.WriteTranscript(taskID, lr.runID, transcript); err != nil {
		return fmt.Errorf("agents: DeliverTurn: write transcript: %w", err)
	}

	td := turnData{
		exit:           exit,
		outManifest:    manifestOf(outParts),
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
	return lr
}

// registerHandle is register keyed by a Handle — used by Wait, where a missing
// or stale entry means auto-resume.
func (h *SimHarness) registerHandle(hd Handle) *liveRun {
	return h.register(hd.TaskID, hd.RunID, hd.Session)
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

// manifestOf lists the delivered filenames in order.
func manifestOf(parts []mime.Part) []string {
	names := make([]string, 0, len(parts))
	for _, p := range parts {
		names = append(names, p.Filename)
	}
	return names
}

// sessionFor is the deterministic session id for a task's continuous
// conversation (docs/05: one task ⇔ one session).
func sessionFor(taskID int64) string {
	return fmt.Sprintf("session-%d", taskID)
}

// transcriptPath is the deterministic workspace-relative transcript location for
// a run; ReadTranscript keys by (taskID, runID), so this is informational.
func transcriptPath(taskID, runID int64) string {
	return fmt.Sprintf("task-%d/transcript-%d.jsonl", taskID, runID)
}
