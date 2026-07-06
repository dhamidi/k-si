package email

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/dhamidi/k-si/cassette"
)

// replayingTransport serves recorded interactions back in order, fully offline —
// it never touches a network. It is how käsi's real JMAP Submit runs in the
// recorded ring: the send code makes its real HTTP calls, and each one is
// answered from the cassette (docs/13). A request that does not match the next
// recorded round-trip is a stale cassette, and staleness is loud, not silent.
type replayingTransport struct {
	mu           sync.Mutex
	interactions []cassette.MailInteraction
	cursor       int
}

// newReplayingTransport serves interactions in the order they were recorded.
func newReplayingTransport(interactions []cassette.MailInteraction) *replayingTransport {
	return &replayingTransport{interactions: interactions}
}

// RoundTrip answers the request from the next recorded interaction. It refuses
// to guess: a request past the end, or one whose method/URL differs from what
// was recorded, is a stale cassette and errors — re-record it via the live
// ring. No network is ever touched.
func (t *replayingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	if t.cursor >= len(t.interactions) {
		t.mu.Unlock()
		return nil, fmt.Errorf("mail cassette exhausted — re-record via the live ring")
	}
	n := t.cursor
	rec := t.interactions[n]
	t.cursor++
	t.mu.Unlock()

	url := req.URL.String()
	if req.Method != rec.Method || url != rec.URL {
		return nil, fmt.Errorf(
			"mail cassette stale (interaction %d): got %s %s, recorded %s %s — re-record via the live ring",
			n, req.Method, url, rec.Method, rec.URL)
	}

	return &http.Response{
		StatusCode: rec.Status,
		Status:     fmt.Sprintf("%d %s", rec.Status, http.StatusText(rec.Status)),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(rec.RespBody)),
		Request:    req,
	}, nil
}
