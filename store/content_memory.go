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
	archive    []ArchiveRow
	nextArch   int64
}

var _ Content = (*MemoryContent)(nil)

func NewMemoryContent() *MemoryContent {
	return &MemoryContent{}
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

func (c *MemoryContent) AddOutbox(row OutboxRow) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

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

func (c *MemoryContent) AddArchive(row ArchiveRow) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if row.SHA256 == "" {
		sum := sha256.Sum256(row.Bytes)
		row.SHA256 = hex.EncodeToString(sum[:])
	}

	c.nextArch++
	row.ID = c.nextArch
	c.archive = append(c.archive, row)
	return row.ID, nil
}

func (c *MemoryContent) ArchiveByTask(taskID int64) ([]ArchiveRow, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var out []ArchiveRow
	for _, r := range c.archive {
		if r.TaskID == taskID {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
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
