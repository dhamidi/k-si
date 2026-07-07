package tasks

import (
	"context"
	"fmt"

	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/store"
	"github.com/dhamidi/k-si/tasks/msg"
)

// "capture-transcript" — copy a run's session transcript from the workspace into the archive
const CaptureTranscript = "capture-transcript"

type CaptureTranscriptPayload struct {
	TaskID         int64  `json:"task_id"`
	RunID          int64  `json:"run_id"`
	TranscriptPath string `json:"transcript_path"`
}

func NewCaptureTranscript(p CaptureTranscriptPayload) runtime.Cmd {
	return runtime.NewCmd(CaptureTranscript, p)
}

func registerCaptureTranscript(mod *runtime.Module) {
	runtime.HandleCmd(mod, CaptureTranscript, captureTranscriptEffect)
}

func captureTranscriptEffect(ctx context.Context, e Edges, p CaptureTranscriptPayload,
	emit runtime.Emit) error {

	b, err := e.Work.ReadTranscript(p.TaskID, p.RunID)
	if err != nil {
		return err
	}
	// AddArchive is idempotent on (task_id, filename), so a re-driven harvest
	// re-archives no duplicate row — the property that makes this reconcilable
	// (decision-013).
	if _, err := e.Content.AddArchive(store.ArchiveRow{
		TaskID:      p.TaskID,
		AgentRun:    p.RunID,
		Kind:        "transcript",
		Filename:    fmt.Sprintf("transcript-%d.jsonl", p.RunID),
		ContentType: "application/jsonl",
		Bytes:       b,
	}); err != nil {
		return err
	}

	// Clear the transcript HarvestJob LAST, once the archive write landed. A crash
	// before this leaves the job pending, so restart's replay rebuilds it and the
	// harvest-reconcile source re-drives the capture — recovering a transcript that
	// an inline effect would have lost forever (decision-013).
	emit(msg.NewMarkHarvested(msg.MarkHarvestedPayload{RunID: p.RunID, Kind: HarvestTranscript}))
	return nil
}
