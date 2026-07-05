package email

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dhamidi/k-si/mime"
)

// SpoolMail is an outbound edge that writes each message to a spool directory as
// a .eml file instead of transmitting it — the stand-in for the JMAP sender while
// only a read-only Fastmail token exists (the send path can't be exercised live).
// It is a real, inspectable edge: `serve` assembles replies straight into the
// spool, so the whole pipeline (mail in → agent → reply) can be watched end to
// end and each outgoing message read off disk. Swapping in JMAP later is a
// one-line assembly change (docs/04, docs/12).
//
// Naming a file by the message's Message-ID makes spooling idempotent on it,
// exactly like the durable send queue's exactly-once guarantee (docs/03): a
// resend of the same reply overwrites the same file rather than duplicating it.
type SpoolMail struct {
	dir string
	mu  sync.Mutex
}

var _ Mail = (*SpoolMail)(nil)

// NewSpoolMail writes outgoing messages into dir (created on first use).
func NewSpoolMail(dir string) *SpoolMail { return &SpoolMail{dir: dir} }

// Submit writes one assembled RFC 5322 message to <dir>/<message-id>.eml.
func (s *SpoolMail) Submit(ctx context.Context, raw []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, spoolName(raw)), raw, 0o644)
}

// List returns the spooled filenames, sorted — for inspecting what was sent.
func (s *SpoolMail) List() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".eml") {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// spoolName derives a stable, filesystem-safe .eml filename from the message's
// Message-ID (so it's idempotent and greppable), falling back to a content hash.
func spoolName(raw []byte) string {
	stem := ""
	if m, err := mime.Parse(raw); err == nil {
		stem = sanitize(m.Header.Get("Message-ID"))
	}
	if stem == "" {
		sum := sha256.Sum256(raw)
		stem = hex.EncodeToString(sum[:8])
	}
	return stem + ".eml"
}

func sanitize(messageID string) string {
	messageID = strings.Trim(messageID, "<>")
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, messageID)
}
