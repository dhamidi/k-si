package email

import (
	"sort"

	"github.com/dhamidi/k-si/runtime"
)

// Model is the email slice of the application model (docs/15): the initiator
// allowlist (email's spam boundary, docs/04) and email's view of the outbox
// send queue — ids and status, deterministically ordered so the model marshals
// stably for the replay-convergence check (docs/13). The MIME bytes themselves
// live in the content store, not here.
type Model struct {
	Allowlist []string      `json:"allowlist"`
	Outbox    []OutboxEntry `json:"outbox"`
	// PollCursor is the JMAP Email state the inbox poller last processed — the
	// high-water mark it resumes from. It is advanced only through record-poll-state
	// (never a private variable), so a restart replays it and picks up mail that
	// arrived while käsi was offline instead of re-anchoring to "now" (decision-018).
	// Absent on pre-decision-018 log entries, so it decodes as "" and replay stays
	// convergent (docs/13).
	PollCursor string `json:"poll_cursor"`
}

// OutboxEntry is email's model of one queued reply. Its status drives
// reconciliation: a "pending" entry keeps a send-email source alive until the
// mail edge has transmitted it and mark-email-sent flips it to "sent" (docs/03).
type OutboxEntry struct {
	OutboxID  int64  `json:"outbox_id"`
	TaskID    int64  `json:"task_id"`
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

// allows reports whether addr is on the initiator allowlist.
func (m Model) allows(addr string) bool {
	for _, a := range m.Allowlist {
		if a == addr {
			return true
		}
	}
	return false
}

// IsAllowed reports whether addr is on the initiator allowlist — the exported
// read serve uses to seed the allowlist without re-logging existing entries.
func IsAllowed(v runtime.View, addr string) bool {
	return runtime.Slice[Model](v, "email").allows(addr)
}

// withAllowed returns the allowlist with addr added (sorted, no duplicate).
func withAllowed(list []string, addr string) []string {
	for _, a := range list {
		if a == addr {
			return list
		}
	}
	next := append(append([]string(nil), list...), addr)
	sort.Strings(next)
	return next
}

// withoutAllowed returns the allowlist with addr removed.
func withoutAllowed(list []string, addr string) []string {
	next := make([]string, 0, len(list))
	for _, a := range list {
		if a != addr {
			next = append(next, a)
		}
	}
	return next
}

// PollCursor returns the JMAP high-water mark the inbox poller should resume from
// — the exported read serve seeds the poll loop with on boot, so a restart picks
// up where the log left off instead of "now" (decision-018). Empty before the
// first poll, which correctly anchors an initial deployment to "now".
func PollCursor(v runtime.View) string {
	return runtime.Slice[Model](v, "email").PollCursor
}

// PendingOutbox returns email's still-unsent outbox entries — the exported pure
// read the reconciliation subscription turns into send-email sources (docs/03).
func PendingOutbox(v runtime.View) []OutboxEntry {
	s := runtime.Slice[Model](v, "email")
	var out []OutboxEntry
	for _, e := range s.Outbox {
		if e.Status == "pending" {
			out = append(out, e)
		}
	}
	return out
}
