package credentials

import (
	"time"

	"github.com/dhamidi/k-si/runtime"
)

// Model is the credentials slice of the application model (docs/15): a name-only
// audit trail of secret mutations. The VALUES live in the secrets edge and never
// touch the log (decision-004); this records only that a reference was set or
// removed, and when — so "which credential changed when" is replayable.
type Model struct {
	Events []Event `json:"events"`
}

// Event is one recorded secret mutation: the reference, the operation, and the
// time it was applied (from meta.Time, the runtime clock — deterministic in
// tests). Never a value.
type Event struct {
	Ref string    `json:"ref"`
	Op  string    `json:"op"` // "set" | "removed"
	At  time.Time `json:"at"`
}

const (
	OpSet     = "set"
	OpRemoved = "removed"
)

// slice reads the credentials Model out of a whole-model View.
func slice(v runtime.View) Model {
	return runtime.Slice[Model](v, "credentials")
}

// Recent returns up to n most-recent audit events, newest first — the trail the
// /secrets page shows beside the live list (docs/06). A copy, so callers never
// alias model-owned events. n <= 0 returns all.
func Recent(v runtime.View, n int) []Event {
	all := slice(v).Events
	out := make([]Event, len(all))
	copy(out, all)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}
