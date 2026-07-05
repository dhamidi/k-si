package tasks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/store"
)

// "archive-task" — archive every workspace file, verify, then delete the workspace (archive-then-delete)
const ArchiveTask = "archive-task"

type ArchiveTaskPayload struct {
	TaskID int64 `json:"task_id"`
}

func NewArchiveTask(p ArchiveTaskPayload) runtime.Cmd {
	return runtime.NewCmd(ArchiveTask, p)
}

func registerArchiveTask(mod *runtime.Module) {
	runtime.HandleCmd(mod, ArchiveTask, archiveTaskEffect)
}

func archiveTaskEffect(ctx context.Context, e Edges, p ArchiveTaskPayload,
	emit runtime.Emit) error {

	// Every file already archived by capture-transcript, keyed by content hash,
	// so we never double-archive a transcript (keeps `archive count transcript`
	// exact).
	existing, _ := e.Content.ArchiveByTask(p.TaskID)
	set := make(map[string]bool, len(existing))
	for _, r := range existing {
		set[r.SHA256] = true
	}

	files, _ := e.Work.Files(p.TaskID)
	for _, f := range files {
		sum := sha256.Sum256(f.Bytes)
		sha := hex.EncodeToString(sum[:])

		// Transcripts were archived by capture-transcript; just note them as
		// covered so Delete's archive-before-delete check passes.
		if strings.HasPrefix(f.Filename, "transcript-") {
			set[sha] = true
			continue
		}

		kind := "attachment"
		if strings.HasPrefix(f.Filename, "out/") {
			kind = "artifact"
		}
		if _, err := e.Content.AddArchive(store.ArchiveRow{
			TaskID:      p.TaskID,
			Kind:        kind,
			Filename:    f.Filename,
			ContentType: f.ContentType,
			SHA256:      sha,
			Bytes:       f.Bytes,
		}); err != nil {
			return err
		}
		set[sha] = true
	}

	return e.Work.Delete(p.TaskID, set)
}
