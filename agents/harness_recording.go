package agents

import (
	"context"
	"strings"
	"sync"

	"github.com/dhamidi/k-si/cassette"
	"github.com/dhamidi/k-si/workspace"
)

// RecordingHarness wraps a real Harness (the Claude twin) and tees each turn
// into a HarnessCassette during a live run (docs/13): the in/ the system laid,
// the out/ the agent left, the verbatim transcript, and how it exited. It is
// interface-transparent — Start/Resume/Signal delegate untouched, and Wait
// returns inner's Result unchanged after capturing the turn. The runner Saves
// the captured turns after a green live run, minting a cassette the recorded
// ring can replay.
type RecordingHarness struct {
	inner Harness
	work  workspace.Workspace
	mu    sync.Mutex
	turns []cassette.HarnessTurn
}

var _ Harness = (*RecordingHarness)(nil)

// NewRecordingHarness wraps inner over the same workspace the turn writes into.
func NewRecordingHarness(inner Harness, work workspace.Workspace) *RecordingHarness {
	return &RecordingHarness{inner: inner, work: work}
}

// Start delegates to the wrapped harness unchanged.
func (h *RecordingHarness) Start(ctx context.Context, taskID, runID int64, env map[string]string) (Handle, error) {
	return h.inner.Start(ctx, taskID, runID, env)
}

// Resume delegates to the wrapped harness unchanged.
func (h *RecordingHarness) Resume(ctx context.Context, taskID, runID int64, session string, env map[string]string) (Handle, error) {
	return h.inner.Resume(ctx, taskID, runID, session, env)
}

// Signal delegates to the wrapped harness unchanged.
func (h *RecordingHarness) Signal(ctx context.Context, hd Handle) error {
	return h.inner.Signal(ctx, hd)
}

// IsLive forwards to the wrapped harness — the decorator holds no registry of
// its own, so liveness is whatever the inner (real) harness reports.
func (h *RecordingHarness) IsLive(hd Handle) bool {
	return h.inner.IsLive(hd)
}

// Wait blocks on the wrapped harness, then captures the finished turn — inputs,
// outputs, transcript, and result — before returning inner's Result unchanged.
func (h *RecordingHarness) Wait(ctx context.Context, hd Handle) Result {
	res := h.inner.Wait(ctx, hd)

	turn := cassette.HarnessTurn{
		TaskID:      hd.TaskID,
		RunID:       hd.RunID,
		Session:     hd.Session,
		Exit:        res.Exit,
		Stopped:     res.Stopped,
		OutManifest: res.OutManifest,
		In:          map[string][]byte{},
		Out:         map[string][]byte{},
	}

	if files, err := h.work.Files(hd.TaskID); err == nil {
		for _, p := range files {
			if name := strings.TrimPrefix(p.Filename, "in/"); name != p.Filename {
				turn.In[name] = p.Bytes
			} else if name := strings.TrimPrefix(p.Filename, "out/"); name != p.Filename {
				turn.Out[name] = p.Bytes
			}
		}
	}
	turn.Transcript, _ = h.work.ReadTranscript(hd.TaskID, hd.RunID)

	h.mu.Lock()
	h.turns = append(h.turns, turn)
	h.mu.Unlock()

	return res
}

// Turns returns a copy of the captured turns — the runner Saves them into a
// HarnessCassette after a green live run.
func (h *RecordingHarness) Turns() []cassette.HarnessTurn {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]cassette.HarnessTurn, len(h.turns))
	copy(out, h.turns)
	return out
}
