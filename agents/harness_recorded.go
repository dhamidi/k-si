package agents

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/dhamidi/k-si/cassette"
	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/workspace"
)

// RecordedHarness is the third Harness twin (docs/13): it replays a captured
// HarnessCassette instead of a scripted turn (SimHarness) or a real subprocess
// (Claude). Each turn is triggered per task by an external DeliverRecorded call
// — the recorded ring's runner — using the SAME rendezvous as SimHarness, so a
// stimulus settles the whole turn before returning. The out/, transcript, and
// exit come from the cassette; the inputs the system lays into in/ are asserted
// against the recording, so a stale cassette fails loud (docs/13).
type RecordedHarness struct {
	mu    sync.Mutex
	cond  *sync.Cond // broadcast when a run is registered, so DeliverRecorded can wait
	work  workspace.Workspace
	runs  map[int64]*liveRun               // keyed by taskID (ephemeral), REUSED from harness_sim.go
	queue map[int64][]cassette.HarnessTurn // per-task recorded turns, popped in order
}

var _ Harness = (*RecordedHarness)(nil)

// NewRecordedHarness builds the recorded harness over a shared workspace and a
// captured cassette. The cassette's turns are grouped by TaskID into per-task
// queues, order preserved; recorded TaskIDs came from the capture run, and the
// deterministic replay reproduces the same offset-derived ids, so they line up.
// A mismatch surfaces as the staleness error in DeliverRecorded.
func NewRecordedHarness(work workspace.Workspace, c cassette.HarnessCassette) *RecordedHarness {
	h := &RecordedHarness{
		work:  work,
		runs:  make(map[int64]*liveRun),
		queue: make(map[int64][]cassette.HarnessTurn),
	}
	for _, turn := range c.Turns {
		h.queue[turn.TaskID] = append(h.queue[turn.TaskID], turn)
	}
	h.cond = sync.NewCond(&h.mu)
	return h
}

// Start registers a live run for a task's first turn and returns immediately.
func (h *RecordedHarness) Start(ctx context.Context, taskID, runID int64) (Handle, error) {
	session := sessionFor(taskID)
	h.register(taskID, runID, session)
	return Handle{TaskID: taskID, RunID: runID, Session: session}, nil
}

// Resume registers a live run continuing an existing session for a later turn.
func (h *RecordedHarness) Resume(ctx context.Context, taskID, runID int64, session string) (Handle, error) {
	if session == "" {
		session = sessionFor(taskID)
	}
	h.register(taskID, runID, session)
	return Handle{TaskID: taskID, RunID: runID, Session: session}, nil
}

// Wait blocks until DeliverRecorded hands this run its turn, or ctx is cancelled,
// or Signal closes the run. Like SimHarness it AUTO-REGISTERS a missing run as a
// resume before blocking, so a restarted watch subscription reconciles it.
func (h *RecordedHarness) Wait(ctx context.Context, hd Handle) Result {
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
// Stopped.
func (h *RecordedHarness) Signal(ctx context.Context, hd Handle) error {
	h.mu.Lock()
	lr := h.runs[hd.TaskID]
	h.mu.Unlock()
	if lr != nil {
		lr.cancelOnce.Do(func() { close(lr.cancel) })
	}
	return nil
}

// DeliverRecorded plays the next recorded turn for taskID through the rendezvous,
// exactly as DeliverTurn plays a scripted one — but the out/, transcript, and exit
// come from the cassette, and it first asserts the inputs the system laid into in/
// match what was recorded (loud staleness, docs/13).
func (h *RecordedHarness) DeliverRecorded(taskID int64) error {
	// Wait for the run to register, same as SimHarness.DeliverTurn.
	h.mu.Lock()
	for h.runs[taskID] == nil {
		h.cond.Wait()
	}
	lr := h.runs[taskID]
	if len(h.queue[taskID]) == 0 {
		h.mu.Unlock()
		return fmt.Errorf("agents: DeliverRecorded: no recorded turn for task %d — cassette stale, re-record via the live ring", taskID)
	}
	turn := h.queue[taskID][0]
	h.queue[taskID] = h.queue[taskID][1:]
	h.mu.Unlock()

	// Staleness check: the inputs the system laid into in/ must match the ones
	// recorded with this turn, else the replay is driving a different task.
	laidIn, err := h.currentIn(taskID)
	if err != nil {
		return fmt.Errorf("agents: DeliverRecorded: read in/ for task %d: %w", taskID, err)
	}
	if diff := diffInputs(turn.In, laidIn); diff != "" {
		return fmt.Errorf("agents: DeliverRecorded: cassette stale for task %d — inputs laid into in/ differ from the recording; re-record via the live ring: %s", taskID, diff)
	}

	outParts := partsFromMap(turn.Out)
	if err := h.work.WriteOut(taskID, outParts); err != nil {
		return fmt.Errorf("agents: DeliverRecorded: write out: %w", err)
	}
	if err := h.work.WriteTranscript(taskID, lr.runID, turn.Transcript); err != nil {
		return fmt.Errorf("agents: DeliverRecorded: write transcript: %w", err)
	}

	td := turnData{
		exit:           turn.Exit,
		outManifest:    turn.OutManifest,
		transcriptPath: transcriptPath(taskID, lr.runID),
	}
	lr.turn <- td // hand off to the blocked Wait
	<-lr.emitted  // wait until finish-agent-run has been enqueued
	return nil
}

// MarkEmitted is called by the agent-watch subscription right after it emits
// finish-agent-run, releasing the matching DeliverRecorded.
func (h *RecordedHarness) MarkEmitted(hd Handle) {
	h.mu.Lock()
	lr := h.runs[hd.TaskID]
	h.mu.Unlock()
	if lr != nil && lr.runID == hd.RunID {
		lr.emitOnce.Do(func() { close(lr.emitted) })
	}
}

// register creates (or, for the same turn, reuses) the live run for taskID.
func (h *RecordedHarness) register(taskID, runID int64, session string) *liveRun {
	h.mu.Lock()
	defer h.mu.Unlock()
	if lr, ok := h.runs[taskID]; ok && lr.runID == runID {
		return lr
	}
	lr := newLiveRun(runID, session)
	h.runs[taskID] = lr
	h.cond.Broadcast() // wake any DeliverRecorded waiting for this run to register
	return lr
}

// registerHandle is register keyed by a Handle — used by Wait, where a missing
// or stale entry means auto-resume.
func (h *RecordedHarness) registerHandle(hd Handle) *liveRun {
	return h.register(hd.TaskID, hd.RunID, hd.Session)
}

// currentIn reads the files the system laid into in/ for taskID, keyed by the
// filename with the "in/" prefix stripped.
func (h *RecordedHarness) currentIn(taskID int64) (map[string][]byte, error) {
	files, err := h.work.Files(taskID)
	if err != nil {
		return nil, err
	}
	return inputsOf(files), nil
}

// inputsOf keeps the "in/"-prefixed parts and strips the prefix.
func inputsOf(files []mime.Part) map[string][]byte {
	in := map[string][]byte{}
	for _, p := range files {
		if name := strings.TrimPrefix(p.Filename, "in/"); name != p.Filename {
			in[name] = p.Bytes
		}
	}
	return in
}

// partsFromMap turns a filename->bytes map into mime.Parts, sorted by filename
// so WriteOut is deterministic.
func partsFromMap(m map[string][]byte) []mime.Part {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]mime.Part, 0, len(names))
	for _, name := range names {
		parts = append(parts, mime.Part{Filename: name, Bytes: m[name]})
	}
	return parts
}

// diffInputs returns a short human description of how laid differs from want, or
// "" when they are byte-for-byte identical over the same key set.
func diffInputs(want, laid map[string][]byte) string {
	var added, removed, changed []string
	for name := range laid {
		if _, ok := want[name]; !ok {
			added = append(added, name)
		}
	}
	for name, wantBytes := range want {
		laidBytes, ok := laid[name]
		if !ok {
			removed = append(removed, name)
			continue
		}
		if !bytes.Equal(wantBytes, laidBytes) {
			changed = append(changed, name)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(changed)

	var msgs []string
	if len(added) > 0 {
		msgs = append(msgs, "added "+strings.Join(added, ","))
	}
	if len(removed) > 0 {
		msgs = append(msgs, "removed "+strings.Join(removed, ","))
	}
	if len(changed) > 0 {
		msgs = append(msgs, "changed "+strings.Join(changed, ","))
	}
	return strings.Join(msgs, "; ")
}
