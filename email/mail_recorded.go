package email

import (
	"context"
	"sync"

	"github.com/dhamidi/k-si/cassette"
	"github.com/dhamidi/k-si/secrets"
)

// RecordedMail is the offline replay mail edge: käsi's real JMAP Submit code
// runs unchanged, but its HTTP calls are answered from a mail-exchange cassette
// instead of the network (docs/13). This is what lets the outbound path run in
// the recorded ring with no Fastmail account and no live credentials — the
// replaying transport ignores the Authorization header, so a sentinel token
// resolved from the sim secrets is enough to satisfy the send code.
type RecordedMail struct {
	inner *JMAP
	mu    sync.Mutex
	sent  [][]byte
}

var _ Mail = (*RecordedMail)(nil)

// NewRecordedMail builds a JMAP client whose transport replays c's interactions.
// No real credentials are needed offline: the sim secrets hand out a sentinel
// token, which the replaying transport never inspects.
func NewRecordedMail(c cassette.MailCassette) *RecordedMail {
	inner := NewJMAP(
		secrets.NewSim(),
		"secret://fastmail/api-token",
		WithTransport(newReplayingTransport(c.Interactions)),
	)
	return &RecordedMail{inner: inner}
}

// Submit drives the real JMAP send path (answered entirely from the cassette)
// and records the raw message only once that send succeeds, so `outbound` never
// observes a message the submit code rejected against a stale cassette.
func (m *RecordedMail) Submit(ctx context.Context, raw []byte) error {
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
func (m *RecordedMail) Sent() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([][]byte, len(m.sent))
	copy(out, m.sent)
	return out
}
