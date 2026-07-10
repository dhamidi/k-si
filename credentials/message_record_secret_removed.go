package credentials

import (
	"github.com/dhamidi/k-si/credentials/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "record-secret-removed" — log that a secret reference was deleted (name only); emitted after the edge Delete

func registerRecordSecretRemoved(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.RecordSecretRemoved, handleRecordSecretRemoved)
}

func handleRecordSecretRemoved(v runtime.View, s Model, p msg.RecordSecretRemovedPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// Append a name-only removal event (copy-on-write), stamped from the runtime
	// clock. Only the reference is recorded (decision-004).
	s.Events = append(append([]Event(nil), s.Events...), Event{Ref: p.Ref, Op: OpRemoved, At: meta.Time})
	return s, nil
}
