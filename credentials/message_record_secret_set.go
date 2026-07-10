package credentials

import (
	"github.com/dhamidi/k-si/credentials/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "record-secret-set" — log that a secret reference was set/rotated (name only, never the value); the web /secrets handler emits it after the edge write

func registerRecordSecretSet(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.RecordSecretSet, handleRecordSecretSet)
}

func handleRecordSecretSet(v runtime.View, s Model, p msg.RecordSecretSetPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// Append a name-only audit event (copy-on-write). meta.Time is the runtime
	// clock — deterministic in tests. The payload carries only the reference; a
	// value never reaches this module (decision-004).
	s.Events = append(append([]Event(nil), s.Events...), Event{Ref: p.Ref, Op: OpSet, At: meta.Time})
	return s, nil
}
