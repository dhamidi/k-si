package tasks

import (
	"context"
	"strings"

	memorymsg "github.com/dhamidi/k-si/memory/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "capture-memory" — harvest the memory gestures a finished run left in its
// workspace (feature-memory.md): a write to out/memory/<name>.md is a REMEMBER
// (upsert from raw content); a deletion of a provisioned in/memory/<name>.md is a
// FORGET. It runs on every successful finish — a deletion leaves no out/ trace, so
// it cannot be gated on the presence of authored memory.
const CaptureMemory = "capture-memory"

type CaptureMemoryPayload struct {
	TaskID int64 `json:"task_id"`
	RunID  int64 `json:"run_id"`
}

func NewCaptureMemory(p CaptureMemoryPayload) runtime.Cmd {
	return runtime.NewCmd(CaptureMemory, p)
}

func registerCaptureMemory(mod *runtime.Module) {
	runtime.HandleCmd(mod, CaptureMemory, captureMemoryEffect)
}

// memoryOutPrefix / memoryInPrefix are where the agent's memory gestures live,
// relative to task-<id> (as Files yields them): a REMEMBER is a file under
// out/memory/, a surviving note is a file under in/memory/.
const (
	memoryOutPrefix = "out/memory/"
	memoryInPrefix  = "in/memory/"
)

func captureMemoryEffect(ctx context.Context, e Edges, p CaptureMemoryPayload,
	emit runtime.Emit) error {

	files, err := e.Work.Files(p.TaskID)
	if err != nil {
		return err
	}

	// Every out/memory/<name>.md is a remember carrying the RAW file (the reducer
	// derives the description on replay). Track the names so a rewrite can suppress
	// a same-name forget: "the outbox wins ties" (feature-memory.md).
	rewritten := map[string]bool{}
	for _, f := range files {
		name, ok := memoryFileName(f.Filename, memoryOutPrefix)
		if !ok {
			continue
		}
		rewritten[name] = true
		emit(memorymsg.NewRemember(memorymsg.RememberPayload{Name: name, Content: f.Bytes}))
	}

	// The notes that survived in this run's in/memory/ box — what the agent did NOT
	// delete.
	survivors := map[string]bool{}
	for _, f := range files {
		name, ok := memoryFileName(f.Filename, memoryInPrefix)
		if !ok {
			continue
		}
		survivors[name] = true
	}

	// The deletion diff, pinned to THIS run's provisioned set (not the live
	// collection, so a memory another task added mid-run never looks deleted):
	// forgotten = provisioned − survivors − rewritten (feature-memory.md).
	provisioned, err := e.Work.ProvisionedMemory(p.TaskID)
	if err != nil {
		return err
	}
	for _, name := range provisioned {
		if survivors[name] || rewritten[name] {
			continue
		}
		emit(memorymsg.NewForget(memorymsg.ForgetPayload{Name: name}))
	}
	return nil
}

// memoryFileName strips a box prefix ("out/memory/") and the ".md" suffix,
// yielding the memory's name. It accepts only a single flat segment — a memory's
// name is a slug, so a nested path (or the MEMORY.md index at the box root) is not
// a note and is skipped.
func memoryFileName(path, prefix string) (string, bool) {
	rel, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return "", false
	}
	if strings.Contains(rel, "/") {
		return "", false
	}
	name, ok := strings.CutSuffix(rel, ".md")
	if !ok || name == "" {
		return "", false
	}
	return name, true
}
