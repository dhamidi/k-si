package email

import (
	"context"
	"fmt"
	"sync"

	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/store"
)

// SimMail is the in-memory twin of the mail edge (docs/12). It records every
// message käsi transmits (so `outbound` can observe it), delivers inbound mail
// into the inbox content table (so `deliver` drives the same path production
// does), and can be told to fail its next N sends (fault injection, docs/13).
//
// Its state — the sent log and the fail counter — is the OUTSIDE WORLD, so the
// runner keeps one SimMail across a simulated crash: a reply transmitted before
// a crash stays transmitted, and a send that failed stays failed until
// reconciliation retries it (Flow E, docs/10).
type SimMail struct {
	mu       sync.Mutex
	content  store.Content
	sent     [][]byte
	failSend int
}

var _ Mail = (*SimMail)(nil)

// NewSimMail builds the sim mail edge over the content store it delivers inbound
// mail into (the same store the email module reads).
func NewSimMail(content store.Content) *SimMail {
	return &SimMail{content: content}
}

// Submit transmits one assembled message, unless a fault has been scripted for
// the next send — in which case it consumes one failure and errors, leaving the
// outbox row pending for reconciliation to retry (docs/03, docs/13).
func (m *SimMail) Submit(ctx context.Context, raw []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failSend > 0 {
		m.failSend--
		return fmt.Errorf("email: simulated mail send failure")
	}
	m.sent = append(m.sent, append([]byte(nil), raw...))
	return nil
}

// Deliver stores an inbound message as an inbox row and returns its id — the
// production path is the inbox subscription noticing new mail; in the sim ring
// the `deliver` vocabulary calls this directly (docs/04, docs/14).
func (m *SimMail) Deliver(raw []byte) (int64, error) {
	parsed, err := mime.Parse(raw)
	if err != nil {
		return 0, fmt.Errorf("email: deliver: parse: %w", err)
	}
	return m.content.AddInbox(store.InboxRow{
		MessageID: parsed.Header.Get("Message-ID"),
		Recipient: parsed.Header.Get("To"),
		Raw:       append([]byte(nil), raw...),
		Status:    "new",
	})
}

// Sent returns a copy of every message transmitted so far, for the `outbound`
// read (docs/14).
func (m *SimMail) Sent() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([][]byte, len(m.sent))
	copy(out, m.sent)
	return out
}

// FailNext scripts the next n operations on an op to fail (docs/13). Only "send"
// is meaningful today.
func (m *SimMail) FailNext(op string, n int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if op == "send" {
		m.failSend += n
	}
}
