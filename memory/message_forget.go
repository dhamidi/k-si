package memory

import (
	"github.com/dhamidi/k-si/memory/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "forget" — remove a memory by name (idempotent). Emitted by the harvest when an
// agent deletes in/memory/<name>.md and by the owner's /memory web page
// (feature-memory.md).

func registerForget(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.Forget, handleForget)
}

func handleForget(v runtime.View, s Model, p msg.ForgetPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// Drop the entry whose name matches; an absent name is a no-op (idempotent).
	// Copy-on-write so the prior model snapshot stays immutable for lock-free
	// readers and replay (rules/no-inplace-model-mutation.yml).
	i := s.findName(p.Name)
	if i < 0 {
		return s, nil
	}
	next := make([]Memory, 0, len(s.Memories)-1)
	next = append(next, s.Memories[:i]...)
	next = append(next, s.Memories[i+1:]...)
	s.Memories = next
	return s, nil
}
