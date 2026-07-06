// Package cassette holds recordings of reality for the recorded ring
// (docs/13). A cassette is captured, never authored: every file carries
// provenance written by the tool that recorded it, and loading refuses a
// file without it — a hand-written "recording" would poison the ring's
// premise that its bytes are what actually happened.
//
// The first cassette kind is the message log: a complete log of a real (or
// recorded) run, replayed against the current build to prove that old logs
// still fold — unknown tags drop, nothing crashes (docs/01).
package cassette

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/dhamidi/k-si/runtime"
)

// Provenance is the required header: who recorded this, from what, when.
type Provenance struct {
	Kind       string    `json:"kind"`        // "message-log"
	RecordedAt time.Time `json:"recorded_at"` // wall clock of the recording run
	RecordedBy string    `json:"recorded_by"` // the tool, e.g. "kasi test --record"
	Source     string    `json:"source"`      // the script or probe that produced it

	Versions map[string]string `json:"versions,omitempty"` // e.g. {"claude":"1.2.3","git":"<sha>"} — for staleness diagnosis
}

func (p Provenance) validate() error {
	switch {
	case p.Kind == "":
		return fmt.Errorf("provenance has no kind")
	case p.RecordedAt.IsZero():
		return fmt.Errorf("provenance has no recorded_at")
	case p.RecordedBy == "":
		return fmt.Errorf("provenance has no recorded_by")
	case p.Source == "":
		return fmt.Errorf("provenance has no source")
	}
	return nil
}

// Entry is one logged message with the Meta it was stamped with.
type Entry struct {
	Offset  int64           `json:"offset"`
	Cause   int64           `json:"cause,omitempty"`
	Time    time.Time       `json:"time"`
	Tag     string          `json:"tag"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// MessageLog is a loaded message-log cassette. It implements runtime.Log
// for replay only — a cassette is read-only by definition.
type MessageLog struct {
	Provenance Provenance
	Entries    []Entry
}

func (c *MessageLog) Append(runtime.Msg, int64, time.Time) (runtime.Meta, error) {
	return runtime.Meta{}, fmt.Errorf("cassette: a recording is read-only")
}

func (c *MessageLog) Replay(fn func(runtime.Msg, runtime.Meta) error) error {
	for _, e := range c.Entries {
		msg := runtime.Msg{Tag: e.Tag, Payload: e.Payload}
		meta := runtime.Meta{Offset: e.Offset, Cause: e.Cause, Time: e.Time}
		if err := fn(msg, meta); err != nil {
			return err
		}
	}
	return nil
}

// Load reads a message-log cassette: a provenance header line, then one
// entry per line. A file without valid provenance is refused — re-record it
// through the tooling instead of editing it (docs/13).
func Load(path string) (*MessageLog, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)

	if !scanner.Scan() {
		return nil, refusal(path, fmt.Errorf("empty file"))
	}

	var prov Provenance
	if err := json.Unmarshal(scanner.Bytes(), &prov); err != nil {
		return nil, refusal(path, err)
	}
	if err := prov.validate(); err != nil {
		return nil, refusal(path, err)
	}
	if prov.Kind != "message-log" {
		return nil, refusal(path, fmt.Errorf("kind %q is not message-log", prov.Kind))
	}

	c := &MessageLog{Provenance: prov}
	line := 1

	for scanner.Scan() {
		line++
		if len(scanner.Bytes()) == 0 {
			continue
		}

		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		c.Entries = append(c.Entries, e)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return c, nil
}

func refusal(path string, cause error) error {
	return fmt.Errorf(
		"%s: not a captured cassette (%v) — cassettes are captured, never authored; record one with `kasi test --record` (docs/13)",
		path, cause)
}

// Save writes a message-log cassette from a replayable log.
func Save(path string, prov Provenance, log runtime.Log) error {
	if err := prov.validate(); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	w := bufio.NewWriter(file)

	header, err := json.Marshal(prov)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(header))

	err = log.Replay(func(msg runtime.Msg, meta runtime.Meta) error {
		line, err := json.Marshal(Entry{
			Offset: meta.Offset, Cause: meta.Cause, Time: meta.Time,
			Tag: msg.Tag, Payload: msg.Payload,
		})
		if err != nil {
			return err
		}
		fmt.Fprintln(w, string(line))
		return nil
	})
	if err != nil {
		return err
	}

	return w.Flush()
}
