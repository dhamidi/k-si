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
	// PollCursors holds the high-water mark for each NAMED inbound mechanism
	// (forwardemail's IMAP UIDVALIDITY/UID, and future polled providers), keyed by
	// mechanism name so two pollers running at once never overwrite each other's
	// cursor. Fastmail keeps its own PollCursor above for back-compatibility, so
	// existing logs replay unchanged. Advanced only through record-poll-state (with
	// its Mechanism field set), so a restart resumes across the offline gap
	// (decision-018). Absent on older log entries, so it decodes as nil.
	PollCursors map[string]string `json:"poll_cursors,omitempty"`
	// Mechanisms is the set of configured delivery providers, keyed by name
	// (spool, fastmail, forwardemail, …), each independently enabled inbound
	// and/or outbound (decision-023). It carries only configuration and
	// secret:// credential references — never plaintext (decision-004). A map
	// marshals by sorted keys, so it stays replay-convergent without extra
	// ordering. Absent on pre-decision-023 log entries, so it decodes as nil and
	// the readers below tolerate that.
	Mechanisms map[string]Mechanism `json:"mechanisms,omitempty"`
	// OutboundVia names the one mechanism that currently sends käsi's replies.
	// Resolved live in the send-outbox handler and threaded into the send-email
	// command, so changing it takes effect on the next queued reply. Empty means
	// the spool default (see OutboundVia below), so a fresh model and every
	// pre-decision-023 log entry resolve to a working, safe sender.
	OutboundVia string `json:"outbound_via,omitempty"`
}

// Mechanism is one configured delivery provider (decision-023): whether it is
// enabled to receive and/or send, the domain it handles, and secret:// references
// to its credentials. It never holds plaintext — the API token and IMAP password
// live in the secrets store and are resolved at the edge, per use (decision-004).
type Mechanism struct {
	Inbound     bool   `json:"inbound"`
	Outbound    bool   `json:"outbound"`
	Domain      string `json:"domain,omitempty"`
	SendCredRef string `json:"send_cred_ref,omitempty"`
	RecvCredRef string `json:"recv_cred_ref,omitempty"`
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

// OutboundVia returns the name of the mechanism that should send käsi's replies.
// It is the exported read the send-outbox handler resolves at send time, so a
// change to the active sender applies to the next queued reply (decision-023). An
// unset value resolves to "spool": a fresh model, and every log written before
// mechanisms existed, sends through the safe development spool rather than nothing.
func OutboundVia(v runtime.View) string {
	return runtime.Slice[Model](v, "email").activeSender()
}

// activeSender is the spool-defaulting resolution shared by the exported reader
// and the OutboundViaName method other modules read through.
func (m Model) activeSender() string {
	if m.OutboundVia == "" {
		return "spool"
	}
	return m.OutboundVia
}

// OutboundViaName exposes the active sender to another module without that module
// importing this package (which imports tasks, so tasks cannot import back). The
// consumer defines a one-method interface and reads the email slice through it;
// email.OutboundVia is the in-package reader for handlers here.
func (m Model) OutboundViaName() string {
	return m.activeSender()
}

// OutboundViaRaw returns the stored active-sender value, empty when never set —
// the "unset" signal the boot guarded seed checks, distinct from OutboundVia which
// collapses unset to "spool" for callers that just want a working sender.
func OutboundViaRaw(v runtime.View) string {
	return runtime.Slice[Model](v, "email").OutboundVia
}

// MechanismOf returns the configuration of the named mechanism and whether it is
// configured at all. Tolerates a nil map (returns the zero Mechanism, false), so
// it is safe on a fresh or pre-decision-023 model.
func MechanismOf(v runtime.View, name string) (Mechanism, bool) {
	m, ok := runtime.Slice[Model](v, "email").Mechanisms[name]
	return m, ok
}

// InboundEnabled reports whether the named mechanism is configured to receive
// mail — the gate each inbound poller checks per tick, so a mechanism that is off
// (or absent) is polled as a no-op (decision-023).
func InboundEnabled(v runtime.View, name string) bool {
	m, ok := runtime.Slice[Model](v, "email").Mechanisms[name]
	return ok && m.Inbound
}

// InboundCursor returns the high-water mark a NAMED inbound poller (forwardemail
// and future polled mechanisms) should resume from — the per-mechanism counterpart
// of PollCursor, which stays fastmail's. Empty before the first poll, which
// correctly anchors an initial deployment to "now".
func InboundCursor(v runtime.View, name string) string {
	return runtime.Slice[Model](v, "email").PollCursors[name]
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
