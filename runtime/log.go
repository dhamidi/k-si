package runtime

import "time"

// Log is the append-only message log — the source of truth for state
// (docs/03). A message is appended before it is applied; startup folds the
// entire log through the same handlers with effects suppressed (docs/01).
type Log interface {
	// Append stores a message with its causation and arrival time and
	// returns the stamped Meta; Offset is the logical clock.
	Append(msg Msg, cause int64, at time.Time) (Meta, error)

	// Replay yields every logged message in offset order.
	Replay(fn func(Msg, Meta) error) error
}
