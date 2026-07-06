package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/runtime"
)

// "lay-in-answers" — write a UI request's collected inputs into the task
// workspace in/: the non-secret text/choice values as answers.json, and each
// uploaded file resolved from its archive id (Flow C). Secrets are NOT laid in —
// they are resolved into the harness environment at the agent edge (decision-004).
const LayInAnswers = "lay-in-answers"

type LayInAnswersPayload struct {
	TaskID   int64             `json:"task_id"`
	Values   map[string]string `json:"values"`
	FileRefs map[string]int64  `json:"file_refs"`
}

func NewLayInAnswers(p LayInAnswersPayload) runtime.Cmd {
	return runtime.NewCmd(LayInAnswers, p)
}

func registerLayInAnswers(mod *runtime.Module) {
	runtime.HandleCmd(mod, LayInAnswers, layInAnswersEffect)
}

func layInAnswersEffect(ctx context.Context, e Edges, p LayInAnswersPayload,
	emit runtime.Emit) error {

	var parts []mime.Part

	if len(p.Values) > 0 {
		b, err := json.Marshal(p.Values)
		if err != nil {
			return fmt.Errorf("tasks: lay-in-answers: marshal values: %w", err)
		}
		parts = append(parts, mime.Part{
			Filename:    "answers.json",
			ContentType: "application/json",
			Bytes:       b,
		})
	}

	// Deterministic field order so the workspace write is reproducible.
	fields := make([]string, 0, len(p.FileRefs))
	for field := range p.FileRefs {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		row, err := e.Content.ArchiveByID(p.FileRefs[field])
		if err != nil {
			return fmt.Errorf("tasks: lay-in-answers: archive %d: %w", p.FileRefs[field], err)
		}
		parts = append(parts, mime.Part{
			Filename:    row.Filename,
			ContentType: row.ContentType,
			Bytes:       row.Bytes,
		})
	}

	if err := e.Work.Create(p.TaskID); err != nil {
		return err
	}
	return e.Work.LayIn(p.TaskID, parts)
}
