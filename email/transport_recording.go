package email

import (
	"bytes"
	"io"
	"net/http"
	"sync"

	"github.com/dhamidi/k-si/cassette"
)

// recordingTransport wraps a real RoundTripper and captures every round-trip it
// carries — method, URL, and both bodies — into a slice a live capture then
// saves as a mail-exchange cassette (docs/13). The Authorization header is
// never read here, so the Bearer token cannot leak into the recording.
type recordingTransport struct {
	inner        http.RoundTripper
	mu           sync.Mutex
	interactions []cassette.MailInteraction
}

// newRecordingTransport wraps inner, defaulting to http.DefaultTransport so a
// nil inner still reaches the live provider.
func newRecordingTransport(inner http.RoundTripper) *recordingTransport {
	if inner == nil {
		inner = http.DefaultTransport
	}
	return &recordingTransport{inner: inner}
}

// RoundTrip buffers the request body so both inner and the recording can read
// it, carries the request over inner, then buffers the response body so both
// the recording and the caller see it. Only method, URL, and bodies are stored.
func (t *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var reqBuf []byte
	if req.Body != nil {
		var err error
		reqBuf, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(reqBuf))
	}

	resp, err := t.inner.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	var respBuf []byte
	if resp.Body != nil {
		respBuf, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
	}

	t.mu.Lock()
	t.interactions = append(t.interactions, cassette.MailInteraction{
		Method:   req.Method,
		URL:      req.URL.String(),
		ReqBody:  reqBuf,
		Status:   resp.StatusCode,
		RespBody: respBuf,
	})
	t.mu.Unlock()

	resp.Body = io.NopCloser(bytes.NewReader(respBuf))
	return resp, nil
}

// captured returns a copy of everything recorded so far.
func (t *recordingTransport) captured() []cassette.MailInteraction {
	t.mu.Lock()
	defer t.mu.Unlock()

	out := make([]cassette.MailInteraction, len(t.interactions))
	copy(out, t.interactions)
	return out
}
