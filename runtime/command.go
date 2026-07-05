package runtime

import (
	"encoding/json"
	"fmt"
)

// Cmd is a description of an effect, returned by a handler and interpreted
// by the runtime (docs/01). Handlers describe; the runtime performs.
type Cmd struct {
	Tag     string          `json:"tag"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// NewCmd builds a command from a tag and a payload value; domain packages
// wrap it in constructors so tags are never spelled at call sites (docs/15).
func NewCmd(tag string, payload any) Cmd {
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("runtime: cannot marshal payload for command %q: %v", tag, err))
	}

	return Cmd{Tag: tag, Payload: raw}
}
