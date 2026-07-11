package email

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dhamidi/k-si/secrets"
)

// ForwardEmail is the ForwardEmail outbound edge: it takes one fully assembled
// message and hands it to ForwardEmail's REST API for delivery. The whole
// RFC 5322 message — From, To, Subject, and the threading headers — goes over as
// a single raw field, so ForwardEmail sends exactly what was built. ForwardEmail
// reads the sending domain from the From header and signs the message with DKIM
// for that domain, which means a verified domain on the paid tier is needed to
// actually send. The API token is resolved from the secrets store on each send,
// so the plaintext never outlives the call.
type ForwardEmail struct {
	http        *http.Client
	secrets     secrets.Secrets
	sendCredRef string
}

var _ Mail = (*ForwardEmail)(nil)

// forwardEmailURL is the ForwardEmail send endpoint. It is a whole literal, not
// assembled from parts, so the URL is easy to read and audit.
const forwardEmailURL = "https://api.forwardemail.net/v1/emails"

// ForwardEmailOption tunes the edge at construction. Options exist so the
// recorded ring can swap the transport (record real traffic, or replay it
// offline) without the send path knowing.
type ForwardEmailOption func(*ForwardEmail)

// WithForwardEmailTransport swaps the http RoundTripper — a recording transport
// captures real ForwardEmail traffic to a cassette, a replaying one serves it
// back offline. The 30s timeout is preserved.
func WithForwardEmailTransport(rt http.RoundTripper) ForwardEmailOption {
	return func(c *ForwardEmail) {
		c.http = &http.Client{Timeout: 30 * time.Second, Transport: rt}
	}
}

// NewForwardEmail builds the ForwardEmail edge. sendCredRef is the secret://
// reference to the API token, resolved through sec at each send. Options are
// applied after the defaults, so the plain two-argument call still yields the
// live client.
func NewForwardEmail(sec secrets.Secrets, sendCredRef string, opts ...ForwardEmailOption) *ForwardEmail {
	c := &ForwardEmail{
		http:        &http.Client{Timeout: 30 * time.Second},
		secrets:     sec,
		sendCredRef: sendCredRef,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Submit transmits one assembled RFC 5322 message through ForwardEmail: the full
// message is posted as the raw form field, and ForwardEmail derives the DKIM
// signing domain from the From header. The API token is resolved here, at the
// edge, and used as the Basic-auth username with an empty password.
func (c *ForwardEmail) Submit(ctx context.Context, raw []byte) error {
	token, err := c.secrets.Resolve(ctx, c.sendCredRef)
	if err != nil {
		return fmt.Errorf("forwardemail: resolve token: %w", err)
	}

	form := url.Values{}
	form.Set("raw", string(raw))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, forwardEmailURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(token, "")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("forwardemail: submit: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("forwardemail: submit: HTTP %d: %s", resp.StatusCode, snippet(resp.Body))
	}
	return nil
}
