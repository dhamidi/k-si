package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/dhamidi/k-si/runtime"
)

// SQLiteLog is the real message log (docs/03): append-only rows in the main
// database, ordered by id — the logical clock. The in-memory twin
// (MemoryLog) carries the simulation ring; this carries production and the
// runner's --log sqlite pass.
type SQLiteLog struct {
	db *sql.DB
}

const messageLogSchema = `
CREATE TABLE IF NOT EXISTS message_log (
  id         INTEGER PRIMARY KEY,          -- monotonic, = replay order
  tag        TEXT    NOT NULL,             -- imperative, e.g. "route-email"
  payload    BLOB    NOT NULL,             -- JSON; references, never secrets
  cause_id   INTEGER,                      -- message that caused this one
  created_at TEXT    NOT NULL              -- RFC3339; recorded, used on replay
);
`

// OpenSQLiteLog opens (creating if needed) the message log in the main
// database file. One writer — the reducer — so no contention tuning is
// needed; WAL keeps readers (future content tables) unblocked.
func OpenSQLiteLog(path string) (*SQLiteLog, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	if _, err := db.Exec(messageLogSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create message_log: %w", err)
	}

	return &SQLiteLog{db: db}, nil
}

func (l *SQLiteLog) Close() error { return l.db.Close() }

func (l *SQLiteLog) Append(msg runtime.Msg, cause int64, at time.Time) (runtime.Meta, error) {
	payload := msg.Payload
	if payload == nil {
		payload = []byte("null")
	}

	var causeValue any
	if cause != 0 {
		causeValue = cause
	}

	result, err := l.db.Exec(
		`INSERT INTO message_log (tag, payload, cause_id, created_at) VALUES (?, ?, ?, ?)`,
		msg.Tag, []byte(payload), causeValue, at.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return runtime.Meta{}, fmt.Errorf("append %q: %w", msg.Tag, err)
	}

	offset, err := result.LastInsertId()
	if err != nil {
		return runtime.Meta{}, err
	}

	return runtime.Meta{Offset: offset, Cause: cause, Time: at}, nil
}

func (l *SQLiteLog) Replay(fn func(runtime.Msg, runtime.Meta) error) error {
	rows, err := l.db.Query(`SELECT id, tag, payload, cause_id, created_at FROM message_log ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			meta    runtime.Meta
			tag     string
			payload []byte
			cause   sql.NullInt64
			created string
		)

		if err := rows.Scan(&meta.Offset, &tag, &payload, &cause, &created); err != nil {
			return err
		}

		meta.Cause = cause.Int64
		meta.Time, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return fmt.Errorf("message %d: bad created_at: %w", meta.Offset, err)
		}

		if err := fn(runtime.Msg{Tag: tag, Payload: payload}, meta); err != nil {
			return err
		}
	}

	return rows.Err()
}
