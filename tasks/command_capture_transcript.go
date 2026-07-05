package tasks

import (
	"context"

	"github.com/dhamidi/k-si/runtime"
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
	return nil
}
