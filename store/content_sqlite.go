package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteContent is the real content store (docs/03): inbound MIME, the
// outbound send queue, and archived files/artifacts/transcripts, kept in the
// main database. The in-memory twin (MemoryContent) carries the simulation
// ring; this carries production and the runner's --log sqlite pass. Mirrors
// SQLiteLog.
type SQLiteContent struct {
	db *sql.DB
}

const contentSchema = `
CREATE TABLE IF NOT EXISTS inbox (
  id          INTEGER PRIMARY KEY,
  message_id  TEXT    NOT NULL UNIQUE,  -- RFC 5322 Message-ID; idempotency key
  raw         BLOB    NOT NULL,         -- original bytes, as received
  parsed      BLOB,                     -- parsed/normalised representation (optional cache)
  recipient   TEXT    NOT NULL,         -- envelope recipient, drives routing
  received_at TEXT    NOT NULL,
  status      TEXT    NOT NULL          -- 'new' | 'routed' | 'ignored'
);

CREATE TABLE IF NOT EXISTS outbox (
  id          INTEGER PRIMARY KEY,
  task_id     INTEGER NOT NULL,
  message_id  TEXT    NOT NULL,         -- our generated Message-ID
  in_reply_to TEXT,                     -- threads the reply
  raw         BLOB    NOT NULL,         -- assembled MIME, ready to send
  status      TEXT    NOT NULL,         -- 'pending' | 'sent' | 'failed'
  created_at  TEXT    NOT NULL,
  sent_at     TEXT
);

CREATE TABLE IF NOT EXISTS archive (
  id           INTEGER PRIMARY KEY,
  task_id      INTEGER NOT NULL,
  kind         TEXT    NOT NULL,        -- 'attachment' | 'artifact' | 'transcript'
  agent_run    INTEGER,                 -- for transcripts/artifacts
  filename     TEXT,
  content_type TEXT,
  sha256       TEXT    NOT NULL,        -- content hash; enables dedup
  bytes        BLOB    NOT NULL,
  created_at   TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS skill (
  id          INTEGER PRIMARY KEY,
  name        TEXT    NOT NULL UNIQUE,  -- referenced by task templates
  description TEXT,
  content     BLOB    NOT NULL,         -- tar of the skill directory tree
  origin      TEXT    NOT NULL,         -- 'ui' | 'agent'
  origin_task INTEGER,                  -- task that authored it, if origin='agent'
  version     INTEGER NOT NULL,         -- bumped on edit; provisioning uses latest
  updated_at  TEXT    NOT NULL
);
`

// OpenSQLiteContent opens (creating if needed) the content tables in the main
// database file. Mirrors OpenSQLiteLog; WAL keeps readers unblocked.
func OpenSQLiteContent(path string) (*SQLiteContent, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	if _, err := db.Exec(contentSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create content tables: %w", err)
	}

	return &SQLiteContent{db: db}, nil
}

var _ Content = (*SQLiteContent)(nil)

func (c *SQLiteContent) Close() error { return c.db.Close() }

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// AddInbox is idempotent on message_id: a pre-check returns the existing row's
// id when the Message-ID is already stored, inserting nothing (docs/03).
func (c *SQLiteContent) AddInbox(row InboxRow) (int64, error) {
	if row.MessageID != "" {
		var id int64
		err := c.db.QueryRow(`SELECT id FROM inbox WHERE message_id = ?`, row.MessageID).Scan(&id)
		if err == nil {
			return id, nil
		}
		if err != sql.ErrNoRows {
			return 0, fmt.Errorf("lookup inbox %q: %w", row.MessageID, err)
		}
	}

	result, err := c.db.Exec(
		`INSERT INTO inbox (message_id, raw, recipient, received_at, status) VALUES (?, ?, ?, ?, ?)`,
		row.MessageID, row.Raw, row.Recipient, row.ReceivedAt.UTC().Format(time.RFC3339Nano), row.Status,
	)
	if err != nil {
		return 0, fmt.Errorf("add inbox %q: %w", row.MessageID, err)
	}
	return result.LastInsertId()
}

func (c *SQLiteContent) Inbox(id int64) (InboxRow, error) {
	row := InboxRow{ID: id}
	var received string
	err := c.db.QueryRow(
		`SELECT message_id, raw, recipient, received_at, status FROM inbox WHERE id = ?`, id,
	).Scan(&row.MessageID, &row.Raw, &row.Recipient, &received, &row.Status)
	if err != nil {
		return InboxRow{}, fmt.Errorf("inbox %d: %w", id, err)
	}
	if row.ReceivedAt, err = time.Parse(time.RFC3339Nano, received); err != nil {
		return InboxRow{}, fmt.Errorf("inbox %d: bad received_at: %w", id, err)
	}
	return row, nil
}

func (c *SQLiteContent) AddOutbox(row OutboxRow) (int64, error) {
	result, err := c.db.Exec(
		`INSERT INTO outbox (task_id, message_id, in_reply_to, raw, status, created_at, sent_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		row.TaskID, row.MessageID, nullableString(row.InReplyTo), row.Raw, row.Status,
		row.CreatedAt.UTC().Format(time.RFC3339Nano), nullableTime(row.SentAt),
	)
	if err != nil {
		return 0, fmt.Errorf("add outbox: %w", err)
	}
	return result.LastInsertId()
}

func (c *SQLiteContent) Outbox(id int64) (OutboxRow, error) {
	row := OutboxRow{ID: id}
	var (
		inReplyTo sql.NullString
		created   string
		sentAt    sql.NullString
	)
	err := c.db.QueryRow(
		`SELECT task_id, message_id, in_reply_to, raw, status, created_at, sent_at FROM outbox WHERE id = ?`, id,
	).Scan(&row.TaskID, &row.MessageID, &inReplyTo, &row.Raw, &row.Status, &created, &sentAt)
	if err != nil {
		return OutboxRow{}, fmt.Errorf("outbox %d: %w", id, err)
	}
	row.InReplyTo = inReplyTo.String
	if row.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return OutboxRow{}, fmt.Errorf("outbox %d: bad created_at: %w", id, err)
	}
	if sentAt.Valid {
		if row.SentAt, err = time.Parse(time.RFC3339Nano, sentAt.String); err != nil {
			return OutboxRow{}, fmt.Errorf("outbox %d: bad sent_at: %w", id, err)
		}
	}
	return row, nil
}

func (c *SQLiteContent) MarkOutboxSent(id int64, at time.Time) error {
	result, err := c.db.Exec(
		`UPDATE outbox SET status = 'sent', sent_at = ? WHERE id = ?`,
		at.UTC().Format(time.RFC3339Nano), id,
	)
	if err != nil {
		return fmt.Errorf("mark outbox %d sent: %w", id, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("outbox %d: not found", id)
	}
	return nil
}

func (c *SQLiteContent) AddArchive(row ArchiveRow) (int64, error) {
	if row.SHA256 == "" {
		sum := sha256.Sum256(row.Bytes)
		row.SHA256 = hex.EncodeToString(sum[:])
	}

	var agentRun any
	if row.AgentRun != 0 {
		agentRun = row.AgentRun
	}

	result, err := c.db.Exec(
		`INSERT INTO archive (task_id, kind, agent_run, filename, content_type, sha256, bytes, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		row.TaskID, row.Kind, agentRun, nullableString(row.Filename), nullableString(row.ContentType),
		row.SHA256, row.Bytes, row.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, fmt.Errorf("add archive: %w", err)
	}
	return result.LastInsertId()
}

func (c *SQLiteContent) ArchiveByTask(taskID int64) ([]ArchiveRow, error) {
	rows, err := c.db.Query(
		`SELECT id, task_id, kind, agent_run, filename, content_type, sha256, bytes, created_at FROM archive WHERE task_id = ? ORDER BY id`, taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ArchiveRow
	for rows.Next() {
		var (
			row         ArchiveRow
			agentRun    sql.NullInt64
			filename    sql.NullString
			contentType sql.NullString
			created     string
		)
		if err := rows.Scan(&row.ID, &row.TaskID, &row.Kind, &agentRun, &filename, &contentType, &row.SHA256, &row.Bytes, &created); err != nil {
			return nil, err
		}
		row.AgentRun = agentRun.Int64
		row.Filename = filename.String
		row.ContentType = contentType.String
		if row.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, fmt.Errorf("archive %d: bad created_at: %w", row.ID, err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (c *SQLiteContent) ArchiveByID(id int64) (ArchiveRow, error) {
	var (
		row         ArchiveRow
		agentRun    sql.NullInt64
		filename    sql.NullString
		contentType sql.NullString
		created     string
	)
	err := c.db.QueryRow(
		`SELECT id, task_id, kind, agent_run, filename, content_type, sha256, bytes, created_at FROM archive WHERE id = ?`, id,
	).Scan(&row.ID, &row.TaskID, &row.Kind, &agentRun, &filename, &contentType, &row.SHA256, &row.Bytes, &created)
	if err != nil {
		return ArchiveRow{}, fmt.Errorf("archive %d: %w", id, err)
	}
	row.AgentRun = agentRun.Int64
	row.Filename = filename.String
	row.ContentType = contentType.String
	if row.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return ArchiveRow{}, fmt.Errorf("archive %d: bad created_at: %w", id, err)
	}
	return row, nil
}

func (c *SQLiteContent) ArchiveCount(taskID int64, kind string) (int, error) {
	var n int
	err := c.db.QueryRow(
		`SELECT COUNT(*) FROM archive WHERE task_id = ? AND kind = ?`, taskID, kind,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("archive count task %d %q: %w", taskID, kind, err)
	}
	return n, nil
}

func skillOriginTask(originTask int64) any {
	if originTask == 0 {
		return nil
	}
	return originTask
}

// AddSkill upserts by name: an existing name bumps that row's version and
// replaces its content and metadata in place, returning the existing id; a fresh
// name inserts and returns the new id (decision-010).
func (c *SQLiteContent) AddSkill(row SkillRow) (int64, error) {
	var (
		id      int64
		version int
	)
	err := c.db.QueryRow(`SELECT id, version FROM skill WHERE name = ?`, row.Name).Scan(&id, &version)
	switch {
	case err == nil:
		_, err = c.db.Exec(
			`UPDATE skill SET description = ?, content = ?, origin = ?, origin_task = ?, version = ?, updated_at = ? WHERE id = ?`,
			nullableString(row.Description), row.Content, row.Origin, skillOriginTask(row.OriginTask), version+1, row.UpdatedAt, id,
		)
		if err != nil {
			return 0, fmt.Errorf("update skill %q: %w", row.Name, err)
		}
		return id, nil
	case err == sql.ErrNoRows:
		result, err := c.db.Exec(
			`INSERT INTO skill (name, description, content, origin, origin_task, version, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			row.Name, nullableString(row.Description), row.Content, row.Origin, skillOriginTask(row.OriginTask), row.Version, row.UpdatedAt,
		)
		if err != nil {
			return 0, fmt.Errorf("add skill %q: %w", row.Name, err)
		}
		return result.LastInsertId()
	default:
		return 0, fmt.Errorf("lookup skill %q: %w", row.Name, err)
	}
}

func (c *SQLiteContent) SkillByID(id int64) (SkillRow, bool, error) {
	return c.scanSkill(c.db.QueryRow(
		`SELECT id, name, description, content, origin, origin_task, version, updated_at FROM skill WHERE id = ?`, id,
	))
}

func (c *SQLiteContent) SkillByName(name string) (SkillRow, bool, error) {
	return c.scanSkill(c.db.QueryRow(
		`SELECT id, name, description, content, origin, origin_task, version, updated_at FROM skill WHERE name = ?`, name,
	))
}

func (c *SQLiteContent) scanSkill(row *sql.Row) (SkillRow, bool, error) {
	var (
		r           SkillRow
		description sql.NullString
		originTask  sql.NullInt64
	)
	err := row.Scan(&r.ID, &r.Name, &description, &r.Content, &r.Origin, &originTask, &r.Version, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return SkillRow{}, false, nil
	}
	if err != nil {
		return SkillRow{}, false, fmt.Errorf("skill: %w", err)
	}
	r.Description = description.String
	r.OriginTask = originTask.Int64
	return r, true, nil
}

func (c *SQLiteContent) AllSkills() ([]SkillRow, error) {
	rows, err := c.db.Query(
		`SELECT id, name, description, content, origin, origin_task, version, updated_at FROM skill ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SkillRow
	for rows.Next() {
		var (
			r           SkillRow
			description sql.NullString
			originTask  sql.NullInt64
		)
		if err := rows.Scan(&r.ID, &r.Name, &description, &r.Content, &r.Origin, &originTask, &r.Version, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Description = description.String
		r.OriginTask = originTask.Int64
		out = append(out, r)
	}
	return out, rows.Err()
}
