// Package store holds the durable stores shared by domains (docs/03). The
// SQLite implementations are the real edges; each has an in-memory twin the
// simulation ring assembles (docs/12).
package store

import (
	"sync"
	"time"

	"github.com/dhamidi/k-si/runtime"
)

// MemoryLog is the in-memory twin of the SQLite message log: append-only,
// offset-ordered, surviving a simulated crash exactly as the file would —
// the runner keeps the log and discards the App (docs/13).
type MemoryLog struct {
	mu      sync.Mutex
	entries []logEntry
}

type logEntry struct {
	msg  runtime.Msg
	meta runtime.Meta
}

func NewMemoryLog() *MemoryLog {
	return &MemoryLog{}
}

func (l *MemoryLog) Append(msg runtime.Msg, cause int64, at time.Time) (runtime.Meta, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	meta := runtime.Meta{Offset: int64(len(l.entries) + 1), Cause: cause, Time: at}
	l.entries = append(l.entries, logEntry{msg: msg, meta: meta})
	return meta, nil
}

func (l *MemoryLog) Replay(fn func(runtime.Msg, runtime.Meta) error) error {
	l.mu.Lock()
	entries := append([]logEntry(nil), l.entries...)
	l.mu.Unlock()

	for _, e := range entries {
		if err := fn(e.msg, e.meta); err != nil {
			return err
		}
	}
	return nil
}

// Tail returns the last n entries as "offset tag" lines with payloads —
// what the runner prints when a scenario fails (docs/14).
func (l *MemoryLog) Tail(n int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	start := len(l.entries) - n
	if start < 0 {
		start = 0
	}

	var lines []string
	for _, e := range l.entries[start:] {
		line := formatEntry(e)
		lines = append(lines, line)
	}
	return lines
}

func formatEntry(e logEntry) string {
	payload := string(e.msg.Payload)
	if payload == "" || payload == "null" {
		payload = "{}"
	}
	return timeAndTag(e.meta, e.msg.Tag) + " " + payload
}

func timeAndTag(meta runtime.Meta, tag string) string {
	return pad(meta.Offset) + " " + tag
}

func pad(offset int64) string {
	const width = 4
	s := ""
	for v := offset; v > 0; v /= 10 {
		s = string(rune('0'+v%10)) + s
	}
	if s == "" {
		s = "0"
	}
	for len(s) < width {
		s = " " + s
	}
	return s
}
