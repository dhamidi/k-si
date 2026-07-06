package tasks

import (
	"github.com/dhamidi/k-si/runtime"
)

// Task — Task struct + state machine + participants + completion token (docs/15)

// TaskID identifies a task; the value is the log offset of its create-task.
type TaskID int64

// Status is the task lifecycle state machine (docs/05).
type Status string

const (
	// Open — created, no agent turn yet requested (unused in Stage 1's happy path).
	Open Status = "open"
	// AwaitingAgent — inbound mail laid in, an agent run is spawning/running.
	AwaitingAgent Status = "awaiting-agent"
	// AwaitingUser — a turn finished; we replied (or stopped) and wait on the human.
	AwaitingUser Status = "awaiting-user"
	// Done — the completion link fired; archived-then-deleted.
	Done Status = "done"
)

// Task is one email-driven unit of work (docs/05). The model holds ids, status,
// and the thread bookkeeping; bytes live in the workspace and the content store.
type Task struct {
	ID       TaskID `json:"id"`
	Status   Status `json:"status"`
	Route    string `json:"route"`
	Template string `json:"template"`
	Subject  string `json:"subject"`
	// Participants: sender first, then CCs, deduped, in insertion order.
	Participants []string `json:"participants"`
	// References: every Message-ID in the thread (inbound + our replies), the set
	// matched against an inbound In-Reply-To / References to thread it back.
	References []string `json:"references"`
	// LastMessageID: the most recent INBOUND Message-ID, used as In-Reply-To on
	// our reply.
	LastMessageID string `json:"last_message_id"`
	// Runs: agent run ids, in order.
	Runs []int64 `json:"runs"`
	// CompletionToken guards the completion link (deterministic in Stage 1).
	CompletionToken string `json:"completion_token"`
	// InboxIDs: the inbox rows laid into this task, in order.
	InboxIDs []int64 `json:"inbox_ids"`
}

// ByThreadKey returns the task whose References contains inReplyTo OR any entry
// of references — how email threads an inbound reply back onto its task (docs/15,
// one-directional cross-domain read owned by tasks).
func ByThreadKey(v runtime.View, inReplyTo string, references []string) (TaskID, bool) {
	m := slice(v)
	for i := range m.Tasks {
		t := m.Tasks[i]
		for _, ref := range t.References {
			if inReplyTo != "" && ref == inReplyTo {
				return t.ID, true
			}
			for _, r := range references {
				if r != "" && ref == r {
					return t.ID, true
				}
			}
		}
	}
	return 0, false
}

// Get returns the task with id, if present — the exported point read.
func Get(v runtime.View, id TaskID) (Task, bool) {
	m := slice(v)
	if i := m.find(id); i >= 0 {
		return m.Tasks[i], true
	}
	return Task{}, false
}

// IsParticipant reports whether addr is one of the task's participants.
func IsParticipant(t Task, addr string) bool {
	for _, p := range t.Participants {
		if p == addr {
			return true
		}
	}
	return false
}

// routeAddr is the fallback from-address a route replies as when no deliverable
// reply-from is configured (the sim ring never sends); real delivery uses the
// configured ReplyFrom (set-reply-from, docs/04).
func routeAddr(route string) string { return route + "@kasi.test" }

// dedup returns addrs with duplicates removed, preserving insertion order.
func dedup(addrs []string) []string {
	seen := make(map[string]bool, len(addrs))
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if a == "" || seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	return out
}
