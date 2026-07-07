package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"
)

// MemoryContent is the in-memory twin of SQLiteContent (docs/03): monotonic
// per-table ids, mutex-guarded, with deterministic (id-ordered) iteration.
// Like the durable store it stands in for, it SURVIVES a simulated crash —
// the runner keeps it while discarding the App (docs/13). All state lives in
// the struct; nothing is global.
type MemoryContent struct {
	mu sync.Mutex

	inbox      []InboxRow
	nextInbox  int64
	outbox     []OutboxRow
	nextOutbox int64
	// archive is the per-task index (the SQLite `archive` table's twin): index rows
	// carry NO bytes — the split from bytes is what exercises the real content-
	// addressed semantics. blobs is the deduped byte store (the `blob` table's twin),
	// one entry per unique sha256. Reads reconstitute a row's Bytes from blobs.
	archive   []ArchiveRow
	nextArch  int64
	blobs     map[string][]byte
	skills    []SkillRow
	nextSkill int64

	// failAddSkill scripts the next N AddSkill calls to fail — the skill harvest's
	// fault-injection knob (decision-013), the store twin of workspace.Memory's
	// failProvisioned. AddSkill is the FIRST durable write store-skill makes and the
	// only op unique to it (its Files/WriteSkills reads are shared with other
	// effects), so failing it leaves the whole store errored before register-skill,
	// which is the crash-mid-store a scenario needs to exercise HarvestPending
	// reconciliation. Like the tables, it survives a simulated crash.
	failAddSkill int

	// failAddArchive scripts the next N AddArchive calls to fail — the transcript
	// harvest's fault-injection knob (decision-013). AddArchive is the durable write
	// capture-transcript makes (its ReadTranscript is a workspace read), and the only
	// AddArchive in the post-finish fan-out, so failing it crashes the transcript
	// harvest mid-flight without touching the memory/skill/reply harvests — the clean
	// seam a transcript-restart-safety scenario needs. Survives a simulated crash.
	failAddArchive int
}

var _ Content = (*MemoryContent)(nil)

func NewMemoryContent() *MemoryContent {
	return &MemoryContent{}
}

// FailNext scripts the next n calls to an op to fail (docs/13) — a sim-only test
// hook mirroring workspace.Memory.FailNext and SimMail.FailNext. Two ops are
// meaningful: "skill" fails AddSkill (the op unique to store-skill) and "archive"
// fails AddArchive (the durable write unique to capture-transcript in the fan-out),
// so a scenario can crash either harvest mid-flight and prove HarvestPending
// reconciliation recovers it (decision-013).
func (c *MemoryContent) FailNext(op string, n int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch op {
	case "skill":
		c.failAddSkill += n
	case "archive":
		c.failAddArchive += n
	}
}

// AddInbox is idempotent on MessageID: a row whose Message-ID already exists
// returns that row's id and inserts nothing (docs/03).
func (c *MemoryContent) AddInbox(row InboxRow) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if row.MessageID != "" {
		for _, r := range c.inbox {
			if r.MessageID == row.MessageID {
				return r.ID, nil
			}
		}
	}

	c.nextInbox++
	row.ID = c.nextInbox
	c.inbox = append(c.inbox, row)
	return row.ID, nil
}

func (c *MemoryContent) Inbox(id int64) (InboxRow, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, r := range c.inbox {
		if r.ID == id {
			return r, nil
		}
	}
	return InboxRow{}, fmt.Errorf("inbox %d: not found", id)
}

// AddOutbox is idempotent on MessageID (the twin of the SQLite pre-check,
// decision-013): a row whose Message-ID already exists returns that row's id and
// inserts nothing, so a re-driven assemble-reply queues no duplicate reply.
func (c *MemoryContent) AddOutbox(row OutboxRow) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if row.MessageID != "" {
		for _, r := range c.outbox {
			if r.MessageID == row.MessageID {
				return r.ID, nil
			}
		}
	}

	c.nextOutbox++
	row.ID = c.nextOutbox
	c.outbox = append(c.outbox, row)
	return row.ID, nil
}

func (c *MemoryContent) Outbox(id int64) (OutboxRow, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, r := range c.outbox {
		if r.ID == id {
			return r, nil
		}
	}
	return OutboxRow{}, fmt.Errorf("outbox %d: not found", id)
}

func (c *MemoryContent) MarkOutboxSent(id int64, at time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.outbox {
		if c.outbox[i].ID == id {
			c.outbox[i].Status = "sent"
			c.outbox[i].SentAt = at
			return nil
		}
	}
	return fmt.Errorf("outbox %d: not found", id)
}

// AddArchive is the behavioural twin of SQLiteContent.AddArchive (decision-013):
// content-addressed and idempotent. The bytes are deduped into the blob map by
// sha256 (stored once, shared across tasks); the index row is inserted idempotently
// on (task_id, filename), returning the existing id when this file is already
// archived for this task. The index row carries no bytes — reads reconstitute them
// from the blob map — so scenarios exercise the same split the durable store does.
func (c *MemoryContent) AddArchive(row ArchiveRow) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.failAddArchive > 0 {
		c.failAddArchive--
		return 0, fmt.Errorf("store: simulated add-archive failure")
	}

	if row.SHA256 == "" {
		sum := sha256.Sum256(row.Bytes)
		row.SHA256 = hex.EncodeToString(sum[:])
	}

	// Dedup the bytes into the blob map — one entry per unique content.
	if c.blobs == nil {
		c.blobs = map[string][]byte{}
	}
	if _, ok := c.blobs[row.SHA256]; !ok {
		c.blobs[row.SHA256] = append([]byte(nil), row.Bytes...)
	}

	// Idempotent index on (task_id, filename).
	if row.Filename != "" {
		for _, r := range c.archive {
			if r.TaskID == row.TaskID && r.Filename == row.Filename {
				return r.ID, nil
			}
		}
	}

	c.nextArch++
	row.ID = c.nextArch
	row.Bytes = nil // the index carries no bytes; reads reconstitute from blobs
	c.archive = append(c.archive, row)
	return row.ID, nil
}

func (c *MemoryContent) ArchiveByTask(taskID int64) ([]ArchiveRow, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var out []ArchiveRow
	for _, r := range c.archive {
		if r.TaskID == taskID {
			r.Bytes = c.blobs[r.SHA256] // reconstitute bytes from the blob store
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (c *MemoryContent) ArchiveByID(id int64) (ArchiveRow, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, r := range c.archive {
		if r.ID == id {
			r.Bytes = c.blobs[r.SHA256] // reconstitute bytes from the blob store
			return r, nil
		}
	}
	return ArchiveRow{}, fmt.Errorf("archive %d: not found", id)
}

// BlobsForFile reports how many DISTINCT blobs the archive indexes for a given
// filename across ALL tasks — a sim-only read backing archive-dedup assertions.
// Content-addressed storage makes this 1 when many tasks archive identical bytes
// under the same name, even though each has its own index row.
func (c *MemoryContent) BlobsForFile(filename string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	seen := map[string]bool{}
	for _, r := range c.archive {
		if r.Filename == filename {
			seen[r.SHA256] = true
		}
	}
	return len(seen)
}

// RowsForFile reports how many archive INDEX rows reference a given filename across
// ALL tasks — the sim-only companion to BlobsForFile, so a scenario can prove both
// tasks' indexes list a file that is stored once (rows > blobs is the dedup).
func (c *MemoryContent) RowsForFile(filename string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	n := 0
	for _, r := range c.archive {
		if r.Filename == filename {
			n++
		}
	}
	return n
}

func (c *MemoryContent) ArchiveCount(taskID int64, kind string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	n := 0
	for _, r := range c.archive {
		if r.TaskID == taskID && r.Kind == kind {
			n++
		}
	}
	return n, nil
}

// AddSkill upserts by name: an existing name bumps that row's version and
// replaces its content and metadata in place, returning the existing id; a fresh
// name inserts and returns the new id (decision-010).
func (c *MemoryContent) AddSkill(row SkillRow) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.failAddSkill > 0 {
		c.failAddSkill--
		return 0, fmt.Errorf("store: simulated add-skill failure")
	}

	for i := range c.skills {
		if c.skills[i].Name == row.Name {
			row.ID = c.skills[i].ID
			row.Version = c.skills[i].Version + 1
			c.skills[i] = row
			return row.ID, nil
		}
	}

	c.nextSkill++
	row.ID = c.nextSkill
	c.skills = append(c.skills, row)
	return row.ID, nil
}

func (c *MemoryContent) SkillByID(id int64) (SkillRow, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, r := range c.skills {
		if r.ID == id {
			return r, true, nil
		}
	}
	return SkillRow{}, false, nil
}

func (c *MemoryContent) SkillByName(name string) (SkillRow, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, r := range c.skills {
		if r.Name == name {
			return r, true, nil
		}
	}
	return SkillRow{}, false, nil
}

func (c *MemoryContent) AllSkills() ([]SkillRow, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := append([]SkillRow(nil), c.skills...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
