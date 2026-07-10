package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteSecrets is the real credential store (docs/06): its own database file,
// values sealed with AES-256-GCM under a key kept outside the database. Resolve
// decrypts in memory per use; the file alone leaks nothing.
type SQLiteSecrets struct {
	db   *sql.DB
	aead cipher.AEAD
}

var _ Secrets = (*SQLiteSecrets)(nil)

const secretSchema = `
CREATE TABLE IF NOT EXISTS secret (
  namespace  TEXT NOT NULL,
  key        TEXT NOT NULL,
  value      BLOB NOT NULL,   -- AES-256-GCM: nonce || ciphertext
  updated_at TEXT NOT NULL,
  PRIMARY KEY (namespace, key)
);
`

// OpenSQLite opens (creating if needed) the secrets database, sealed under the
// given 32-byte key. The key comes from the host, never from this file (docs/06).
func OpenSQLite(path string, key []byte) (*SQLiteSecrets, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secrets: key must be 32 bytes (AES-256): %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("secrets: open %s: %w", path, err)
	}
	if _, err := db.Exec(secretSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("secrets: create schema: %w", err)
	}
	return &SQLiteSecrets{db: db, aead: aead}, nil
}

func (s *SQLiteSecrets) Close() error { return s.db.Close() }

// Set stores (or replaces) the plaintext for a secret:// URL, sealing it. This
// is a management operation — the settings UI (docs/08) or the `kasi secret`
// subcommand — never a handler or a replay path, so fresh randomness is fine.
func (s *SQLiteSecrets) Set(url, plaintext string) error {
	ns, key, err := parseURL(url)
	if err != nil {
		return err
	}

	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	sealed := s.aead.Seal(nonce, nonce, []byte(plaintext), nil)

	_, err = s.db.Exec(
		`INSERT INTO secret (namespace, key, value, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(namespace, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		ns, key, sealed, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("secrets: store %s: %w", url, err)
	}
	return nil
}

// Resolve turns a secret:// URL into its plaintext (docs/06). Called only inside
// an effect, at the instant of use; the returned value is dropped when the
// effect returns and never re-enters the model or log.
func (s *SQLiteSecrets) Resolve(ctx context.Context, url string) (string, error) {
	ns, key, err := parseURL(url)
	if err != nil {
		return "", err
	}

	var sealed []byte
	err = s.db.QueryRowContext(ctx,
		`SELECT value FROM secret WHERE namespace = ? AND key = ?`, ns, key,
	).Scan(&sealed)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("secrets: no secret at %s", url)
	}
	if err != nil {
		return "", err
	}

	nonceSize := s.aead.NonceSize()
	if len(sealed) < nonceSize {
		return "", fmt.Errorf("secrets: %s is corrupt", url)
	}
	plaintext, err := s.aead.Open(nil, sealed[:nonceSize], sealed[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("secrets: cannot decrypt %s (wrong key?): %w", url, err)
	}
	return string(plaintext), nil
}

// List returns the secret:// URLs present, in sorted order — references only,
// never values. It backs `kasi secret ls` and the settings UI's "a secret
// exists" display (docs/06, docs/08).
func (s *SQLiteSecrets) List() ([]string, error) {
	rows, err := s.db.Query(`SELECT namespace, key FROM secret ORDER BY namespace, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var ns, key string
		if err := rows.Scan(&ns, &key); err != nil {
			return nil, err
		}
		urls = append(urls, URL(ns, key))
	}
	return urls, rows.Err()
}

// Entries returns every stored secret as a reference plus its last-set time, in
// sorted order — references only, never values (docs/06). It backs the /secrets
// management page, which shows what exists and when it was last set. A malformed
// updated_at is tolerated (zero time) rather than failing the whole listing.
func (s *SQLiteSecrets) Entries() ([]Entry, error) {
	rows, err := s.db.Query(`SELECT namespace, key, updated_at FROM secret ORDER BY namespace, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var ns, key, updated string
		if err := rows.Scan(&ns, &key, &updated); err != nil {
			return nil, err
		}
		at, _ := time.Parse(time.RFC3339, updated)
		entries = append(entries, Entry{Ref: URL(ns, key), UpdatedAt: at})
	}
	return entries, rows.Err()
}

// Delete removes the secret at url, sealing off a credential the operator has
// retired (docs/06). Deleting an absent secret is a no-op success — idempotent,
// so a retry or a double-submit is harmless.
func (s *SQLiteSecrets) Delete(url string) error {
	ns, key, err := parseURL(url)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM secret WHERE namespace = ? AND key = ?`, ns, key); err != nil {
		return fmt.Errorf("secrets: delete %s: %w", url, err)
	}
	return nil
}

// LoadKey returns the 32-byte encryption key from the host: the base64 value of
// $KASI_SECRETS_KEY if set, otherwise a per-deployment key file beside the
// databases (created on first use, 0600). Either way the key lives OUTSIDE the
// secrets database (docs/06 invariant 5). A file "only the process can read" is
// an explicitly sanctioned source there.
func LoadKey(stateDir string) ([]byte, error) {
	if env := os.Getenv("KASI_SECRETS_KEY"); env != "" {
		key, err := base64.StdEncoding.DecodeString(env)
		if err != nil {
			return nil, fmt.Errorf("secrets: KASI_SECRETS_KEY must be base64: %w", err)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("secrets: KASI_SECRETS_KEY must decode to 32 bytes, got %d", len(key))
		}
		return key, nil
	}

	path := filepath.Join(stateDir, "secrets.key")
	if key, err := os.ReadFile(path); err == nil {
		if len(key) != 32 {
			return nil, fmt.Errorf("secrets: %s must be 32 bytes, got %d", path, len(key))
		}
		return key, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("secrets: write key file: %w", err)
	}
	return key, nil
}
