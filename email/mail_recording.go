package email

import (
	"context"
	"net/http"
	"sync"

	"github.com/dhamidi/k-si/cassette"
	"github.com/dhamidi/k-si/secrets"
)

// RecordingMail is the live-capture mail edge: it is the real JMAP client with a
// recording transport spliced under it, so every message it actually sends to
// Fastmail is captured as mail-exchange interactions the tooling then saves to a
// cassette (docs/13). It is used once, against the live ring, to mint the tape
// RecordedMail later replays offline.
type RecordingMail struct {
	inner *JMAP
	rec   *recordingTransport
	mu    sync.Mutex
	sent  [][]byte
}

var _ Mail = (*RecordingMail)(nil)

// NewRecordingMail builds the real Fastmail edge over a recording transport
// wrapping the default transport, so its traffic is both real and captured.
func NewRecordingMail(sec secrets.Secrets, tokenURL string) *RecordingMail {
	rec := newRecordingTransport(http.DefaultTransport)
	return &RecordingMail{
		inner: NewJMAP(sec, tokenURL, WithTransport(rec)),
		rec:   rec,
	}
}

// Submit transmits the raw message for real through the inner JMAP client and
// records it only once that send succeeds, so a capture observes what was
// actually sent rather than what was merely attempted.
func (m *RecordingMail) Submit(ctx context.Context, raw []byte) error {
	if err := m.inner.Submit(ctx, raw); err != nil {
		return err
	}

	m.mu.Lock()
	m.sent = append(m.sent, append([]byte(nil), raw...))
	m.mu.Unlock()

	return nil
}

// Sent returns a copy of every message transmitted so far, for the `outbound`
// read (docs/14).
func (m *RecordingMail) Sent() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([][]byte, len(m.sent))
	copy(out, m.sent)
	return out
}

// Interactions returns a copy of the HTTP round-trips captured so far, ready to
// be written to a mail-exchange cassette.
func (m *RecordingMail) Interactions() []cassette.MailInteraction {
	return m.rec.captured()
}
