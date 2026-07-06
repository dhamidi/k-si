package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dhamidi/k-si/secrets"
)

// JMAP is the real mail edge (docs/04): käsi's Fastmail integration, spoken
// directly over the stdlib http client and encoding/json — no SDK. The Bearer
// token is resolved from the secrets store at the edge, per operation (docs/06),
// so plaintext never outlives the call. Session discovery is done once and
// cached.
type JMAP struct {
	http       *http.Client
	sessionURL string
	secrets    secrets.Secrets
	tokenURL   string

	apiURL      string
	downloadURL string
	uploadURL   string
	accountID   string
}

var _ Mail = (*JMAP)(nil)

const (
	defaultSessionURL = "https://api.fastmail.com/jmap/session"

	capCore       = "urn:ietf:params:jmap:core"
	capMail       = "urn:ietf:params:jmap:mail"
	capSubmission = "urn:ietf:params:jmap:submission"
)

// Inbound is one fetched inbound message: its raw RFC 5322 bytes plus the
// envelope facts route-email needs (docs/04).
type Inbound struct {
	Raw       []byte
	MessageID string
	Recipient string
}

// JMAPOption tunes the edge at construction. Options exist so the recorded ring
// can swap the transport (record real traffic, or replay it offline) and point
// session discovery at a stand-in URL, without the send path knowing (docs/13).
type JMAPOption func(*JMAP)

// WithTransport swaps the http RoundTripper — a recording transport captures
// real Fastmail traffic to a cassette, a replaying one serves it back offline.
// The 30s timeout is preserved.
func WithTransport(rt http.RoundTripper) JMAPOption {
	return func(c *JMAP) {
		c.http = &http.Client{Timeout: 30 * time.Second, Transport: rt}
	}
}

// WithSessionURL overrides the session-discovery URL (for replay/tests).
func WithSessionURL(u string) JMAPOption {
	return func(c *JMAP) {
		c.sessionURL = u
	}
}

// NewJMAP builds the Fastmail edge. tokenURL is the secret:// reference to the
// API token (secret://fastmail/api-token), resolved through sec at each use.
// Options are applied after the defaults, so the plain two-argument call still
// yields the live client.
func NewJMAP(sec secrets.Secrets, tokenURL string, opts ...JMAPOption) *JMAP {
	c := &JMAP{
		http:       &http.Client{Timeout: 30 * time.Second},
		sessionURL: defaultSessionURL,
		secrets:    sec,
		tokenURL:   tokenURL,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// --- session ------------------------------------------------------------------

type sessionInfo struct {
	APIURL          string            `json:"apiUrl"`
	DownloadURL     string            `json:"downloadUrl"`
	UploadURL       string            `json:"uploadUrl"`
	PrimaryAccounts map[string]string `json:"primaryAccounts"`
}

func (c *JMAP) token(ctx context.Context) (string, error) {
	tok, err := c.secrets.Resolve(ctx, c.tokenURL)
	if err != nil {
		return "", fmt.Errorf("jmap: resolve token: %w", err)
	}
	return tok, nil
}

// ensureSession discovers apiUrl, the primary mail account, and downloadUrl once
// (docs/04). Cached, so later calls skip the round trip.
func (c *JMAP) ensureSession(ctx context.Context, token string) error {
	if c.apiURL != "" {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.sessionURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("jmap: session: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jmap: session: HTTP %d: %s", resp.StatusCode, snippet(resp.Body))
	}

	var s sessionInfo
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return fmt.Errorf("jmap: session decode: %w", err)
	}
	if s.APIURL == "" || s.PrimaryAccounts[capMail] == "" {
		return fmt.Errorf("jmap: session has no mail account")
	}
	c.apiURL, c.downloadURL, c.uploadURL = s.APIURL, s.DownloadURL, s.UploadURL
	c.accountID = s.PrimaryAccounts[capMail]
	return nil
}

// --- request/response ---------------------------------------------------------

// invocation is one JMAP method call or response: [name, args, callId].
type invocation struct {
	Name   string
	Args   json.RawMessage
	CallID string
}

func (i invocation) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{i.Name, i.Args, i.CallID})
}

func (i *invocation) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw) != 3 {
		return fmt.Errorf("jmap: invocation must have 3 elements, got %d", len(raw))
	}
	if err := json.Unmarshal(raw[0], &i.Name); err != nil {
		return err
	}
	i.Args = raw[1]
	return json.Unmarshal(raw[2], &i.CallID)
}

// call posts a batch of method calls and returns their responses, in order.
func (c *JMAP) call(ctx context.Context, token string, using []string, calls ...invocation) ([]invocation, error) {
	body, err := json.Marshal(map[string]any{"using": using, "methodCalls": calls})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jmap: call: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jmap: call: HTTP %d: %s", resp.StatusCode, snippet(resp.Body))
	}

	var out struct {
		MethodResponses []invocation `json:"methodResponses"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("jmap: response decode: %w", err)
	}
	for _, r := range out.MethodResponses {
		if r.Name == "error" {
			return nil, fmt.Errorf("jmap: method error: %s", r.Args)
		}
	}
	return out.MethodResponses, nil
}

// invoke builds one method call with JSON args.
func invoke(name string, args map[string]any, callID string) invocation {
	raw, _ := json.Marshal(args)
	return invocation{Name: name, Args: raw, CallID: callID}
}

// download fetches a blob's bytes via the session download URL template.
func (c *JMAP) download(ctx context.Context, token, blobID, name string) ([]byte, error) {
	url := c.downloadURL
	url = strings.ReplaceAll(url, "{accountId}", c.accountID)
	url = strings.ReplaceAll(url, "{blobId}", blobID)
	url = strings.ReplaceAll(url, "{name}", name)
	url = strings.ReplaceAll(url, "{type}", "application/octet-stream")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jmap: download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jmap: download: HTTP %d: %s", resp.StatusCode, snippet(resp.Body))
	}
	return io.ReadAll(resp.Body)
}

// --- reading (validated live with a read-only token) --------------------------

// Recent fetches up to limit of the most recent messages in the Inbox, each with
// its raw RFC 5322 bytes — the inbound path (JMAP → raw MIME) end to end. It is a
// read-only operation, safe to run against a live account.
func (c *JMAP) Recent(ctx context.Context, limit int) ([]Inbound, error) {
	token, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.ensureSession(ctx, token); err != nil {
		return nil, err
	}

	inboxID, err := c.inboxID(ctx, token)
	if err != nil {
		return nil, err
	}

	// Email/query the newest messages, then Email/get their blobId + envelope by
	// referring back to the query result (a JMAP result reference).
	responses, err := c.call(ctx, token, []string{capCore, capMail},
		invoke("Email/query", map[string]any{
			"accountId": c.accountID,
			"filter":    map[string]any{"inMailbox": inboxID},
			"sort":      []any{map[string]any{"property": "receivedAt", "isAscending": false}},
			"limit":     limit,
		}, "q"),
		invoke("Email/get", map[string]any{
			"accountId":  c.accountID,
			"#ids":       map[string]any{"resultOf": "q", "name": "Email/query", "path": "/ids"},
			"properties": []string{"id", "blobId", "messageId", "to"},
		}, "g"),
	)
	if err != nil {
		return nil, err
	}

	var got struct {
		List []struct {
			BlobID    string   `json:"blobId"`
			MessageID []string `json:"messageId"`
			To        []struct {
				Email string `json:"email"`
			} `json:"to"`
		} `json:"list"`
	}
	if err := decodeResult(responses, "Email/get", &got); err != nil {
		return nil, err
	}

	var out []Inbound
	for _, e := range got.List {
		raw, err := c.download(ctx, token, e.BlobID, "message.eml")
		if err != nil {
			return nil, err
		}
		out = append(out, Inbound{
			Raw:       raw,
			MessageID: bracket(first(e.MessageID)),
			Recipient: firstEmail(e.To),
		})
	}
	return out, nil
}

// Fetch returns inbound messages that arrived since a prior JMAP state, plus the
// new state to poll from next — the incremental inbound path (docs/04). The first
// call (empty sinceState) returns the CURRENT state and no messages, so a poller
// only ever processes mail that arrives after it starts, never the whole mailbox.
func (c *JMAP) Fetch(ctx context.Context, sinceState string) (msgs []Inbound, newState string, err error) {
	token, err := c.token(ctx)
	if err != nil {
		return nil, "", err
	}
	if err := c.ensureSession(ctx, token); err != nil {
		return nil, "", err
	}

	if sinceState == "" {
		state, err := c.mailState(ctx, token)
		return nil, state, err
	}

	responses, err := c.call(ctx, token, []string{capCore, capMail},
		invoke("Email/changes", map[string]any{
			"accountId":  c.accountID,
			"sinceState": sinceState,
			"maxChanges": 50,
		}, "ch"),
		invoke("Email/get", map[string]any{
			"accountId":  c.accountID,
			"#ids":       map[string]any{"resultOf": "ch", "name": "Email/changes", "path": "/created"},
			"properties": []string{"id", "blobId", "messageId", "to"},
		}, "g"),
	)
	if err != nil {
		return nil, "", err
	}

	var changed struct {
		NewState string `json:"newState"`
	}
	if err := decodeResult(responses, "Email/changes", &changed); err != nil {
		return nil, "", err
	}

	var got struct {
		List []struct {
			BlobID    string   `json:"blobId"`
			MessageID []string `json:"messageId"`
			To        []struct {
				Email string `json:"email"`
			} `json:"to"`
		} `json:"list"`
	}
	if err := decodeResult(responses, "Email/get", &got); err != nil {
		return nil, "", err
	}

	for _, e := range got.List {
		raw, err := c.download(ctx, token, e.BlobID, "message.eml")
		if err != nil {
			return nil, "", err
		}
		msgs = append(msgs, Inbound{Raw: raw, MessageID: bracket(first(e.MessageID)), Recipient: firstEmail(e.To)})
	}
	return msgs, changed.NewState, nil
}

// mailState reads the current Email state (the high-water mark to poll from).
func (c *JMAP) mailState(ctx context.Context, token string) (string, error) {
	responses, err := c.call(ctx, token, []string{capCore, capMail},
		invoke("Email/get", map[string]any{"accountId": c.accountID, "ids": []string{}}, "s"),
	)
	if err != nil {
		return "", err
	}
	var got struct {
		State string `json:"state"`
	}
	if err := decodeResult(responses, "Email/get", &got); err != nil {
		return "", err
	}
	return got.State, nil
}

func (c *JMAP) inboxID(ctx context.Context, token string) (string, error) {
	responses, err := c.call(ctx, token, []string{capCore, capMail},
		invoke("Mailbox/query", map[string]any{
			"accountId": c.accountID,
			"filter":    map[string]any{"role": "inbox"},
		}, "mq"),
	)
	if err != nil {
		return "", err
	}
	var q struct {
		IDs []string `json:"ids"`
	}
	if err := decodeResult(responses, "Mailbox/query", &q); err != nil {
		return "", err
	}
	if len(q.IDs) == 0 {
		return "", fmt.Errorf("jmap: no Inbox mailbox")
	}
	return q.IDs[0], nil
}

// --- sending (coded; not live-validated with a read-only token) ---------------

// Submit transmits one assembled RFC 5322 message via JMAP: upload the bytes as
// a blob, Email/import it into Drafts, then EmailSubmission/set to send. Requires
// a send-capable token; with the read-only token used in development this returns
// the provider's permission error, which is the correct, honest behaviour until a
// send-capable credential is provisioned (docs/04).
func (c *JMAP) Submit(ctx context.Context, raw []byte) error {
	token, err := c.token(ctx)
	if err != nil {
		return err
	}
	if err := c.ensureSession(ctx, token); err != nil {
		return err
	}

	blobID, err := c.upload(ctx, token, raw)
	if err != nil {
		return err
	}

	drafts, sent, identity, err := c.sendContext(ctx, token)
	if err != nil {
		return err
	}

	_, err = c.call(ctx, token, []string{capCore, capMail, capSubmission},
		invoke("Email/import", map[string]any{
			"accountId": c.accountID,
			"emails": map[string]any{
				"msg": map[string]any{
					"blobId":     blobID,
					"mailboxIds": map[string]any{drafts: true},
					"keywords":   map[string]any{"$draft": true, "$seen": true},
				},
			},
		}, "import"),
		invoke("EmailSubmission/set", map[string]any{
			"accountId": c.accountID,
			"create": map[string]any{
				"sub": map[string]any{"emailId": "#msg", "identityId": identity},
			},
			// Keep the sent reply in käsi's own mailbox so it shows up and threads
			// with the conversation (docs/04): move it from Drafts to Sent and clear
			// the draft flag, rather than destroying it after the send.
			"onSuccessUpdateEmail": map[string]any{
				"#sub": map[string]any{
					"mailboxIds": map[string]any{sent: true},
					"keywords":   map[string]any{"$seen": true},
				},
			},
		}, "submit"),
	)
	return err
}

// upload posts the raw message to the blob store and returns its blobId.
func (c *JMAP) upload(ctx context.Context, token string, raw []byte) (string, error) {
	url := strings.ReplaceAll(c.uploadURL, "{accountId}", c.accountID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "message/rfc822")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("jmap: upload: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("jmap: upload: HTTP %d: %s", resp.StatusCode, snippet(resp.Body))
	}

	var up struct {
		BlobID string `json:"blobId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&up); err != nil {
		return "", fmt.Errorf("jmap: upload decode: %w", err)
	}
	if up.BlobID == "" {
		return "", fmt.Errorf("jmap: upload returned no blobId")
	}
	return up.BlobID, nil
}

// sendContext looks up the Drafts mailbox and the sending identity a submission
// needs (docs/04). The first identity is used; matching From to a specific
// identity is a refinement for when multiple sending addresses exist.
func (c *JMAP) sendContext(ctx context.Context, token string) (draftsID, sentID, identityID string, err error) {
	responses, err := c.call(ctx, token, []string{capCore, capMail, capSubmission},
		invoke("Mailbox/get", map[string]any{"accountId": c.accountID}, "mg"),
		invoke("Identity/get", map[string]any{"accountId": c.accountID}, "ig"),
	)
	if err != nil {
		return "", "", "", err
	}

	var boxes struct {
		List []struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"list"`
	}
	if err := decodeResult(responses, "Mailbox/get", &boxes); err != nil {
		return "", "", "", err
	}
	for _, b := range boxes.List {
		switch b.Role {
		case "drafts":
			draftsID = b.ID
		case "sent":
			sentID = b.ID
		}
	}
	if draftsID == "" {
		return "", "", "", fmt.Errorf("jmap: no Drafts mailbox")
	}
	if sentID == "" {
		return "", "", "", fmt.Errorf("jmap: no Sent mailbox")
	}

	var ident struct {
		List []struct {
			ID string `json:"id"`
		} `json:"list"`
	}
	if err := decodeResult(responses, "Identity/get", &ident); err != nil {
		return "", "", "", err
	}
	if len(ident.List) == 0 {
		return "", "", "", fmt.Errorf("jmap: no sending identity")
	}
	return draftsID, sentID, ident.List[0].ID, nil
}

// --- helpers ------------------------------------------------------------------

// decodeResult finds the named method response and unmarshals its args.
func decodeResult(responses []invocation, method string, into any) error {
	for _, r := range responses {
		if r.Name == method {
			return json.Unmarshal(r.Args, into)
		}
	}
	return fmt.Errorf("jmap: no %s in response", method)
}

func snippet(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 512))
	return strings.TrimSpace(string(b))
}

func first(ss []string) string {
	if len(ss) > 0 {
		return ss[0]
	}
	return ""
}

// bracket wraps a JMAP messageId in the RFC 5322 angle brackets the JMAP spec
// strips, so In-Reply-To/References built from it actually match — Gmail won't
// thread otherwise. Empty or already-bracketed ids pass through unchanged.
func bracket(id string) string {
	if id == "" || strings.HasPrefix(id, "<") {
		return id
	}
	return "<" + id + ">"
}

func firstEmail(to []struct {
	Email string `json:"email"`
}) string {
	if len(to) > 0 {
		return to[0].Email
	}
	return ""
}
