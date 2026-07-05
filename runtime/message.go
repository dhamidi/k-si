package runtime

import (
	"encoding/json"
	"fmt"
	"time"
)

// Msg is a runtime message: a stable, imperative tag plus a complete,
// serialisable payload (docs/01). Everything a handler needs is in the
// payload or in Meta; a handler never reaches out for more.
type Msg struct {
	Tag     string          `json:"tag"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// NewMsg builds a message from a tag and a payload value. Constructors in
// domain packages wrap this so a tag literal appears exactly once, in the
// file that owns it (docs/15).
func NewMsg(tag string, payload any) Msg {
	raw, err := json.Marshal(payload)
	if err != nil {
		// Payloads are plain structs of plain values; failing to marshal one
		// is a programming error, not a runtime condition.
		panic(fmt.Sprintf("runtime: cannot marshal payload for %q: %v", tag, err))
	}

	return Msg{Tag: tag, Payload: raw}
}

// Meta is what the runtime stamps onto a message as it is logged: its log
// offset (the logical clock — ids derive from it), the offset of the message
// that caused it, and the arrival time (recorded once, replayed verbatim).
type Meta struct {
	Offset int64     `json:"offset"`
	Cause  int64     `json:"cause,omitempty"`
	Time   time.Time `json:"time"`
}

// Emit is how edges (effects and subscriptions) feed results back into the
// reducer — always as complete messages, never by touching the model.
type Emit func(Msg)
