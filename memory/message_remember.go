package memory

import (
	"github.com/dhamidi/k-si/memory/msg"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/skilltree"
)

// "remember" — upsert a memory by name from its RAW file; the reducer derives the
// description (store raw, derive on replay). Emitted by the harvest
// (tasks/capture-memory) and by the owner's /memory web form (feature-memory.md).

func registerRemember(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.Remember, handleRemember)
}

func handleRemember(v runtime.View, s Model, p msg.RememberPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// Derive the description from the RAW memory file HERE, in the pure reducer, so
	// every replay re-parses it with the current parser — a parser fix or a new
	// frontmatter-derived field is free on the next replay, no migration. The log
	// holds the raw fact; the model holds the current code's reading of it. The
	// frontmatter reader is the same one skills uses (feature-memory.md).
	_, description := skilltree.Frontmatter(p.Content)

	entry := Memory{
		Name:        p.Name,
		Content:     append([]byte(nil), p.Content...),
		Description: description,
	}

	// A name is UNIQUE (feature-memory.md): remembering an existing name replaces
	// that memory's raw content rather than appending a duplicate. Copy-on-write so
	// the prior model stays untouched for lock-free readers and replay
	// (rules/no-inplace-model-mutation.yml).
	next := append([]Memory(nil), s.Memories...)
	if i := s.findName(p.Name); i >= 0 {
		next[i] = entry
	} else {
		next = append(next, entry)
	}
	s.Memories = next
	return s, nil
}
