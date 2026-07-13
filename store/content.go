package store

import "time"

// Content is the durable store for bytes the model refers to by id (docs/03):
// inbound MIME (inbox), the outbound send queue (outbox), and archived files,
// artifacts, and transcripts (archive). The model holds ids + status; the
// bytes live here. Two twins implement it: MemoryContent (simulation ring)
// and SQLiteContent (production and the runner's --log sqlite pass).
type Content interface {
	// AddInbox stores an inbound MIME row. It is idempotent on MessageID:
	// a duplicate Message-ID returns the existing id and inserts nothing.
	AddInbox(InboxRow) (int64, error)
	Inbox(id int64) (InboxRow, error)

	AddOutbox(OutboxRow) (int64, error)
	Outbox(id int64) (OutboxRow, error)
	// MarkOutboxSent flips a pending row to 'sent' and records sent_at.
	MarkOutboxSent(id int64, at time.Time) error

	AddArchive(ArchiveRow) (int64, error)
	ArchiveByTask(taskID int64) ([]ArchiveRow, error)
	// ArchiveByID returns a single archive row by id, backing lay-in-answers
	// resolving an uploaded file from its reference (Flow C).
	ArchiveByID(id int64) (ArchiveRow, error)
	// ArchiveCount counts archive rows for a task of a given kind, backing
	// `archive count task N transcript`.
	ArchiveCount(taskID int64, kind string) (int, error)

	// AddSkill stores a skill's tree (Flow D, decision-010). Name is UNIQUE: on a
	// name clash it bumps the existing row's version and replaces its content,
	// description, origin metadata, and updated_at, returning the EXISTING id —
	// so a re-authored skill updates in place. Otherwise it inserts and returns
	// the new id.
	AddSkill(SkillRow) (int64, error)
	SkillByID(id int64) (SkillRow, bool, error)
	SkillByName(name string) (SkillRow, bool, error)
	AllSkills() ([]SkillRow, error)
	// DeleteSkill removes a skill's tree by its unique name (Flow D Ask 2). The
	// owner retires a skill from the web UI; this drops the tar so provisioning
	// (which reads AllSkills directly) stops laying it into future runs. Deleting
	// an absent name is a no-op success — idempotent, so a double-submit or retry
	// is harmless.
	DeleteSkill(name string) error
}

// SkillRow is one row of the skill table (docs/03): an agent-authored (or
// UI-authored) Agent Skills directory kept durably, separate from the ephemeral
// workspace, so a skill created in one run survives into the next. Content is a
// tar of the whole tree (decision-010); the model's registry holds only light
// metadata referencing this row by id.
type SkillRow struct {
	ID          int64
	Name        string // UNIQUE; the folder name, matches SKILL.md frontmatter
	Description string
	Content     []byte // tar of the skill directory
	Origin      string // 'ui' | 'agent'
	OriginTask  int64  // task that authored it, if origin='agent'; 0 otherwise
	Version     int    // bumped on re-author; provisioning uses latest
	UpdatedAt   string // RFC3339 timestamp
}

// InboxRow is one row of the inbox table (docs/03): inbound MIME landed by
// mail delivery, referenced by a route-email message via id.
type InboxRow struct {
	ID         int64
	MessageID  string // RFC 5322 Message-ID; idempotency key
	Recipient  string // envelope recipient, drives routing
	Raw        []byte // original bytes, as received
	ReceivedAt time.Time
	Status     string // 'new' | 'routed' | 'ignored'
}

// OutboxRow is one row of the outbox table (docs/03): a durable, crash-safe
// send queue. A handler writes a 'pending' row; the send effect marks it
// 'sent'.
type OutboxRow struct {
	ID        int64
	TaskID    int64
	MessageID string // our generated Message-ID; makes duplicate sends detectable
	InReplyTo string // threads the reply
	Raw       []byte // assembled MIME, ready to send
	Status    string // 'pending' | 'sent' | 'failed'
	CreatedAt time.Time
	SentAt    time.Time
}

// ArchiveRow is one row of the archive index (docs/03): a file kept before its
// task's workspace is deleted. Storage is content-addressed (decision-013) — the
// bytes live ONCE in the blob table keyed by SHA256, and the index references them;
// but this struct carries Bytes at the API boundary (callers still pass and receive
// bytes), so the blob/index split stays internal to the store.
type ArchiveRow struct {
	ID          int64
	TaskID      int64
	AgentRun    int64  // for transcripts/artifacts; 0 if unset
	Kind        string // 'attachment' | 'artifact' | 'transcript'
	Filename    string
	ContentType string
	SHA256      string // hex content hash; computed from Bytes if empty
	Bytes       []byte
	CreatedAt   time.Time
}
