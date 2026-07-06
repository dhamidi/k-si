package main

// `kasi capture-inbox` is ring-3 tooling (docs/13): it captures REAL inbound
// mail from the live Fastmail account into the parse corpus, so
// t/fixtures/mime/*.eml keeps growing from reality rather than from
// hand-authored fixtures — the ones reality wrote carry the headers, encodings
// and multipart nesting that actually break the parser. It is read-only against
// the account (JMAP Recent, no writes) and, like `kasi probe`, is never in the
// merge loop: you run it deliberately, in an environment with credentials. It
// writes nothing but fixture .eml files and their provenance record.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhamidi/k-si/email"
	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/secrets"
)

// captureProvenance mirrors cassette.Provenance's shape (Kind/RecordedBy/Source):
// a captured artifact records who recorded it, from what, and when. Files maps
// each fixture filename to the envelope facts and top-level structure, so a
// reader sees at a glance that these are captured-from-reality and what shape
// each one is.
type captureProvenance struct {
	Kind       string                   `json:"kind"`        // "mime-fixture"
	RecordedBy string                   `json:"recorded_by"` // "kasi capture-inbox"
	Source     string                   `json:"source"`      // the live account (Fastmail Inbox)
	UpdatedAt  time.Time                `json:"updated_at"`
	Files      map[string]captureRecord `json:"files"`
}

// captureRecord is one captured message's provenance.
type captureRecord struct {
	MessageID  string    `json:"message_id"`
	From       string    `json:"from"`
	Subject    string    `json:"subject"`
	Recipient  string    `json:"recipient"`
	CapturedAt time.Time `json:"captured_at"`
	Structure  string    `json:"structure"` // top-level Content-Type, e.g. "multipart/mixed"
}

func runCaptureInbox(args []string) int {
	flags := flag.NewFlagSet("kasi capture-inbox", flag.ExitOnError)
	n := flags.Int("n", 10, "how many recent inbox messages to fetch")
	state := flags.String("state", "data", "state directory holding the secrets database (docs/03)")
	dir := flags.String("dir", "t/fixtures/mime", "corpus output directory for captured .eml fixtures")
	// Flag parsing happens BEFORE any secrets/network access, so `-help`/`-h`
	// print the flags and exit without touching the account.
	flags.Parse(args)

	if *n <= 0 {
		return fail("kasi capture-inbox:", fmt.Errorf("-n must be positive, got %d", *n))
	}

	// Open the secrets store + JMAP client exactly as `kasi serve` does. A clear
	// error if the store or token is missing — this is credential-gated tooling.
	key, err := secrets.LoadKey(*state)
	if err != nil {
		return fail("kasi capture-inbox:", fmt.Errorf("secrets key (is -state %q set up? run `kasi secret`): %w", *state, err))
	}
	sec, err := secrets.OpenSQLite(filepath.Join(*state, "secrets.db"), key)
	if err != nil {
		return fail("kasi capture-inbox:", fmt.Errorf("secrets store (%s): %w", filepath.Join(*state, "secrets.db"), err))
	}
	defer sec.Close()
	jmap := email.NewJMAP(sec, "secret://fastmail/api-token")

	if err := os.MkdirAll(*dir, 0o755); err != nil {
		return fail("kasi capture-inbox:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	msgs, err := jmap.Recent(ctx, *n)
	if err != nil {
		return fail("kasi capture-inbox:", fmt.Errorf("read live inbox (need a valid secret://fastmail/api-token): %w", err))
	}

	prov, err := loadCaptureProvenance(filepath.Join(*dir, "provenance.json"))
	if err != nil {
		return fail("kasi capture-inbox:", err)
	}
	now := time.Now().UTC()

	captured := 0
	for _, m := range msgs {
		parsed, err := mime.Parse(m.Raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "kasi capture-inbox: skip %s: parse: %v\n", m.MessageID, err)
			continue
		}
		subject := parsed.Header.Get("Subject")
		from := parsed.Header.Get("From")
		structure := topStructure(parsed)

		name := captureSlug(subject, from, m.MessageID) + ".eml"
		path := filepath.Join(*dir, name)
		// Stable-overwrite: the same Message-ID always maps to the same filename,
		// so re-running captures reality without duplicating fixtures.
		if err := os.WriteFile(path, m.Raw, 0o644); err != nil {
			return fail("kasi capture-inbox:", err)
		}
		prov.Files[name] = captureRecord{
			MessageID:  m.MessageID,
			From:       from,
			Subject:    subject,
			Recipient:  m.Recipient,
			CapturedAt: now,
			Structure:  structure,
		}
		captured++
		fmt.Printf("captured %s  %s  %s\n", path, structure, subject)
	}

	prov.UpdatedAt = now
	if err := saveCaptureProvenance(filepath.Join(*dir, "provenance.json"), prov); err != nil {
		return fail("kasi capture-inbox:", err)
	}

	fmt.Printf("kasi capture-inbox: captured %d message(s) into %s — read the LIVE inbox (read-only; no mail sent, no money spent).\n", captured, *dir)
	return 0
}

// topStructure is the message's top-level Content-Type media type (the "shape"
// at a glance, e.g. "multipart/mixed"). The parameters (boundary, charset) are
// dropped — only the type/subtype matters for the corpus overview.
func topStructure(m mime.Message) string {
	ct := m.Header.Get("Content-Type")
	if ct == "" {
		return "text/plain"
	}
	return strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
}

// captureSlug derives a stable, filesystem-safe slug from a message: a
// lower-kebab base from the Subject (falling back to From, then a constant),
// disambiguated by a short hash of the Message-ID. It is DETERMINISTIC — the
// same Message-ID always yields the same filename — so re-running overwrites the
// same fixture instead of duplicating it.
func captureSlug(subject, from, messageID string) string {
	base := slugify(subject)
	if base == "" {
		base = slugify(from)
	}
	if base == "" {
		base = "message"
	}
	if len(base) > 60 {
		base = strings.Trim(base[:60], "-")
	}
	sum := sha256.Sum256([]byte(messageID))
	return base + "-" + hex.EncodeToString(sum[:])[:8]
}

// slugify lowercases and reduces a string to [a-z0-9] runs joined by single
// dashes, trimming leading/trailing dashes.
func slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			dash = false
		default:
			if b.Len() > 0 && !dash {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// loadCaptureProvenance reads an existing provenance.json so a re-run merges new
// captures with prior ones rather than clobbering the record. A missing file
// yields a fresh, empty provenance.
func loadCaptureProvenance(path string) (captureProvenance, error) {
	prov := captureProvenance{
		Kind:       "mime-fixture",
		RecordedBy: "kasi capture-inbox",
		Source:     "live Fastmail Inbox (secret://fastmail/api-token)",
		Files:      map[string]captureRecord{},
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return prov, nil
	}
	if err != nil {
		return prov, err
	}
	if err := json.Unmarshal(data, &prov); err != nil {
		return prov, fmt.Errorf("provenance %s: %w", path, err)
	}
	if prov.Files == nil {
		prov.Files = map[string]captureRecord{}
	}
	prov.Kind = "mime-fixture"
	prov.RecordedBy = "kasi capture-inbox"
	prov.Source = "live Fastmail Inbox (secret://fastmail/api-token)"
	return prov, nil
}

func saveCaptureProvenance(path string, prov captureProvenance) error {
	data, err := json.MarshalIndent(prov, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
