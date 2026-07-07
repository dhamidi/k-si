package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
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

// contentSchemaVersion is the current content-DB schema version, tracked in
// PRAGMA user_version. Bumping it and adding a case to applyContentMigration is
// how future schema changes ship; OpenSQLiteContent runs the migration runner
// before creating tables, so an old DB is upgraded in place and a fresh DB is
// stamped at this version and skips every migration (see migrateContent).
const contentSchemaVersion = 1

// contentSchema is the CURRENT (v1) shape. Archival is content-addressed
// (decision-013): each unique blob is stored ONCE in `blob`, keyed by its sha256,
// and `archive` is a per-task index that references the blob by sha256 and carries
// no bytes of its own. UNIQUE(task_id, filename) makes AddArchive idempotent — the
// same file re-archived for the same task is a no-op — which is what lets
// capture-transcript be reconciled. `CREATE TABLE IF NOT EXISTS` makes a FRESH DB
// at this shape directly; an OLD DB reaches it through the v0→v1 migration.
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
  message_id  TEXT    NOT NULL UNIQUE,  -- our generated Message-ID; idempotency key (decision-013)
  in_reply_to TEXT,                     -- threads the reply
  raw         BLOB    NOT NULL,         -- assembled MIME, ready to send
  status      TEXT    NOT NULL,         -- 'pending' | 'sent' | 'failed'
  created_at  TEXT    NOT NULL,
  sent_at     TEXT
);

CREATE TABLE IF NOT EXISTS blob (
  sha256 TEXT PRIMARY KEY,              -- hex content hash; the content address
  bytes  BLOB NOT NULL                  -- the unique bytes, stored ONCE
);

CREATE TABLE IF NOT EXISTS archive (
  id           INTEGER PRIMARY KEY,
  task_id      INTEGER NOT NULL,
  kind         TEXT    NOT NULL,        -- 'attachment' | 'artifact' | 'transcript'
  agent_run    INTEGER,                 -- for transcripts/artifacts
  filename     TEXT,
  content_type TEXT,
  sha256       TEXT    NOT NULL,        -- references blob(sha256); reconstitutes the bytes
  created_at   TEXT    NOT NULL,
  UNIQUE(task_id, filename)             -- one archive row per file per task; idempotency key
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

	// Migrate BEFORE creating tables: an existing OLD-schema DB is upgraded in place
	// (v0→v1 content-addresses the archive), a fresh DB is a clean no-op that just
	// stamps user_version, and a current DB skips entirely. Only then do we run
	// CREATE TABLE IF NOT EXISTS, which materialises the current shape for a fresh DB
	// and is a no-op once the tables exist.
	if err := migrateContent(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate content: %w", err)
	}

	if _, err := db.Exec(contentSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create content tables: %w", err)
	}

	return &SQLiteContent{db: db}, nil
}

// migrateContent brings the content DB up to contentSchemaVersion, keyed on
// PRAGMA user_version — the minimal, crash-safe versioned migration mechanism (the
// project had none before). Each step runs in its own TRANSACTION that also bumps
// user_version, so a crash mid-migration rolls the whole step back and a re-run
// starts it over; a completed step is skipped because user_version has advanced.
// This is idempotent (safe to call on every open) and the mechanism future schema
// changes reuse: bump contentSchemaVersion and add a case to applyContentMigration.
func migrateContent(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	for version < contentSchemaVersion {
		next := version + 1
		vacuum, err := applyContentMigration(db, next)
		if err != nil {
			return fmt.Errorf("v%d→v%d: %w", version, next, err)
		}
		version = next
		// VACUUM reclaims the bytes the migration freed; it CANNOT run inside a
		// transaction, so it runs here, after the step committed. Re-running it is safe.
		if vacuum {
			if _, err := db.Exec(`VACUUM`); err != nil {
				return fmt.Errorf("vacuum after v%d: %w", next, err)
			}
		}
	}
	return nil
}

// applyContentMigration runs the single step that reaches version `to`, in a
// transaction that also advances user_version to `to`. It reports whether the
// caller should VACUUM afterwards (a step that rewrote a table frees pages worth
// reclaiming). A step is written to be a no-op on a fresh DB so a brand-new v1
// database migrates cleanly (nothing to move) rather than erroring.
func applyContentMigration(db *sql.DB, to int) (vacuum bool, err error) {
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	switch to {
	case 1:
		vacuum, err = migrateContentV1(tx)
	default:
		err = fmt.Errorf("no migration defined")
	}
	if err != nil {
		return false, err
	}

	// PRAGMA user_version is transactional (it writes the DB header), so it commits
	// or rolls back atomically with the step above. Not parameterisable — `to` is an
	// int constant, so the format is safe.
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, to)); err != nil {
		return false, fmt.Errorf("set user_version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return vacuum, nil
}

// migrateContentV1 is the archive content-addressing migration (decision-013). An
// OLD archive stored `bytes` inline per row, so the whole memory collection and
// every provisioned skill were duplicated into every task's DB (unbounded, tasks ×
// collection). This dedups the bytes into a single `blob` table keyed by sha256 and
// rebuilds `archive` WITHOUT bytes, referencing the blob by sha256 and enforcing
// UNIQUE(task_id, filename). It self-guards: if the archive has no `bytes` column
// (a fresh DB the CREATE TABLEs will build new-shape, or an already-migrated one)
// there is nothing to move, so it returns cleanly and the caller just adopts v1.
func migrateContentV1(tx *sql.Tx) (bool, error) {
	has, err := archiveHasBytesColumn(tx)
	if err != nil {
		return false, err
	}
	if !has {
		return false, nil // fresh or already new-shaped — nothing to migrate
	}

	steps := []string{
		// The blob store, one row per unique content.
		`CREATE TABLE IF NOT EXISTS blob (sha256 TEXT PRIMARY KEY, bytes BLOB NOT NULL)`,
		// Dedup every existing archived byte-string into it; duplicate content
		// (the same file archived by many tasks) collapses to ONE blob.
		`INSERT OR IGNORE INTO blob(sha256, bytes) SELECT sha256, bytes FROM archive`,
		// Rebuild the index WITHOUT bytes and with UNIQUE(task_id, filename).
		`CREATE TABLE archive_new (
		   id           INTEGER PRIMARY KEY,
		   task_id      INTEGER NOT NULL,
		   kind         TEXT    NOT NULL,
		   agent_run    INTEGER,
		   filename     TEXT,
		   content_type TEXT,
		   sha256       TEXT    NOT NULL,
		   created_at   TEXT    NOT NULL,
		   UNIQUE(task_id, filename)
		 )`,
		// Preserve ids; INSERT OR IGNORE drops any pre-existing duplicate
		// (task_id, filename) rows (the old schema had no such constraint), keeping
		// the earliest.
		`INSERT OR IGNORE INTO archive_new (id, task_id, kind, agent_run, filename, content_type, sha256, created_at)
		   SELECT id, task_id, kind, agent_run, filename, content_type, sha256, created_at FROM archive`,
		`DROP TABLE archive`,
		`ALTER TABLE archive_new RENAME TO archive`,
	}
	for _, s := range steps {
		if _, err := tx.Exec(s); err != nil {
			return false, fmt.Errorf("%s: %w", firstSQLLine(s), err)
		}
	}
	return true, nil
}

// archiveHasBytesColumn reports whether the archive table currently has an inline
// `bytes` column — i.e. it is the OLD, pre-content-addressed shape. PRAGMA
// table_info on a non-existent table yields zero rows (no error), so a fresh DB
// with no archive table reads as false.
func archiveHasBytesColumn(tx *sql.Tx) (bool, error) {
	rows, err := tx.Query(`PRAGMA table_info(archive)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, ctype      string
			dflt             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == "bytes" {
			return true, rows.Err()
		}
	}
	return false, rows.Err()
}

// firstSQLLine returns the first non-blank line of a SQL statement, for readable
// migration error messages.
func firstSQLLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return s
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

// AddOutbox is idempotent on message_id, exactly like AddInbox: a row whose
// (deterministic, per run) Message-ID already exists returns that row's id and
// inserts nothing. This makes a re-driven assemble-reply queue the SAME row rather
// than a duplicate that would send a second copy — the pre-assemble crash window
// decision-013 closes.
func (c *SQLiteContent) AddOutbox(row OutboxRow) (int64, error) {
	if row.MessageID != "" {
		var id int64
		err := c.db.QueryRow(`SELECT id FROM outbox WHERE message_id = ?`, row.MessageID).Scan(&id)
		if err == nil {
			return id, nil
		}
		if err != sql.ErrNoRows {
			return 0, fmt.Errorf("lookup outbox %q: %w", row.MessageID, err)
		}
	}

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

// AddArchive is content-addressed and idempotent (decision-013). It stores the
// bytes ONCE in the blob table keyed by sha256 (INSERT OR IGNORE dedups content
// shared across tasks), then inserts a per-task index row idempotently on
// (task_id, filename) — a pre-check like AddInbox/AddOutbox returns the existing
// id when this file is already archived for this task, so a re-driven
// capture-transcript (or a second archive-task pass) never duplicates a row. The
// ArchiveRow's Bytes stay at the API boundary; the blob/index split is internal.
func (c *SQLiteContent) AddArchive(row ArchiveRow) (int64, error) {
	if row.SHA256 == "" {
		sum := sha256.Sum256(row.Bytes)
		row.SHA256 = hex.EncodeToString(sum[:])
	}

	// Store the bytes once, shared by every task that archives this content.
	if _, err := c.db.Exec(`INSERT OR IGNORE INTO blob(sha256, bytes) VALUES (?, ?)`, row.SHA256, row.Bytes); err != nil {
		return 0, fmt.Errorf("add archive blob: %w", err)
	}

	// Idempotent index on (task_id, filename): already archived → return that id.
	if row.Filename != "" {
		var id int64
		err := c.db.QueryRow(`SELECT id FROM archive WHERE task_id = ? AND filename = ?`, row.TaskID, row.Filename).Scan(&id)
		if err == nil {
			return id, nil
		}
		if err != sql.ErrNoRows {
			return 0, fmt.Errorf("lookup archive (task %d, %q): %w", row.TaskID, row.Filename, err)
		}
	}

	var agentRun any
	if row.AgentRun != 0 {
		agentRun = row.AgentRun
	}

	result, err := c.db.Exec(
		`INSERT INTO archive (task_id, kind, agent_run, filename, content_type, sha256, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		row.TaskID, row.Kind, agentRun, nullableString(row.Filename), nullableString(row.ContentType),
		row.SHA256, row.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, fmt.Errorf("add archive: %w", err)
	}
	return result.LastInsertId()
}

func (c *SQLiteContent) ArchiveByTask(taskID int64) ([]ArchiveRow, error) {
	rows, err := c.db.Query(
		`SELECT a.id, a.task_id, a.kind, a.agent_run, a.filename, a.content_type, a.sha256, b.bytes, a.created_at
		   FROM archive a JOIN blob b ON a.sha256 = b.sha256
		  WHERE a.task_id = ? ORDER BY a.id`, taskID,
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
		`SELECT a.id, a.task_id, a.kind, a.agent_run, a.filename, a.content_type, a.sha256, b.bytes, a.created_at
		   FROM archive a JOIN blob b ON a.sha256 = b.sha256
		  WHERE a.id = ?`, id,
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
