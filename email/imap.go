package email

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/dhamidi/k-si/secrets"
)

// IMAP is a second inbound mail edge for providers käsi reaches over IMAP rather
// than JMAP. It speaks the protocol directly over a TLS connection — no SDK,
// stdlib only — and returns the same Inbound shape the JMAP edge does, so the
// router downstream never learns which edge a message came in on. The login
// password is resolved from the secrets store per poll (docs/06), so plaintext
// never outlives the call.
//
// Only the minimum verbs are implemented: LOGIN, SELECT, UID FETCH, LOGOUT over
// implicit TLS on port 993. No IDLE, no STARTTLS, no second mailbox.
type IMAP struct {
	host        string
	username    string
	secrets     secrets.Secrets
	passwordRef string

	// dial opens the transport to the server. It is a seam: the default is a TLS
	// dialer to host:993, but a recorded ring can swap in a stub that replays
	// captured traffic offline (mirrors the JMAP edge's WithTransport idea).
	dial func(ctx context.Context, host string) (net.Conn, error)
}

// IMAPOption tunes the edge at construction.
type IMAPOption func(*IMAP)

// WithIMAPDial swaps the connection seam — for replay or tests, a stub that
// serves recorded server bytes instead of touching the network.
func WithIMAPDial(fn func(ctx context.Context, host string) (net.Conn, error)) IMAPOption {
	return func(c *IMAP) { c.dial = fn }
}

// NewIMAP builds the IMAP edge. passwordRef is the secret:// reference to the
// account password (resolved through sec at each poll, never at construction).
// Options are applied after the defaults, so the plain call still yields a live
// client dialing host:993 over TLS.
func NewIMAP(sec secrets.Secrets, host, username, passwordRef string, opts ...IMAPOption) *IMAP {
	c := &IMAP{
		host:        host,
		username:    username,
		secrets:     sec,
		passwordRef: passwordRef,
		dial:        dialTLS,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// dialTLS is the default seam: implicit TLS to host:993 with a connect timeout.
func dialTLS(ctx context.Context, host string) (net.Conn, error) {
	d := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 30 * time.Second}}
	return d.DialContext(ctx, "tcp", net.JoinHostPort(host, "993"))
}

// Fetch returns inbound messages that arrived since a prior poll, plus the cursor
// to poll from next — the incremental inbound path, mirroring jmap.Fetch. The
// cursor is an opaque string that rides the käsi log (decision-018): it is
// recorded as a value and replayed, so callers must treat it as a token, not
// inspect it. Its shape is "<uidvalidity>/<uidnext>".
//
// The first call (empty cursor) returns the CURRENT mailbox state and no
// messages, so a fresh deployment anchors to "now" and never ingests the whole
// existing mailbox. If the mailbox's UIDVALIDITY changes under a live cursor the
// mailbox was reset, so the poll re-anchors to the current state rather than
// fetching across the discontinuity.
func (c *IMAP) Fetch(ctx context.Context, cursor string) (msgs []Inbound, next string, err error) {
	pw, err := c.secrets.Resolve(ctx, c.passwordRef)
	if err != nil {
		return nil, "", fmt.Errorf("imap: resolve password: %w", err)
	}

	conn, err := c.dial(ctx, c.host)
	if err != nil {
		return nil, "", fmt.Errorf("imap: dial: %w", err)
	}
	defer conn.Close()

	// Bound the whole exchange by the context deadline, or 60s if none is set.
	deadline := time.Now().Add(60 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)

	s := &imapSession{conn: conn, r: bufio.NewReader(conn)}

	// A clean LOGOUT on the way out, whatever happened; its errors don't matter
	// once we have (or failed to get) the messages.
	defer func() { _, _ = s.do("LOGOUT") }()

	greeting, err := readLine(s.r)
	if err != nil {
		return nil, "", fmt.Errorf("imap: greeting: %w", err)
	}
	if !strings.HasPrefix(greeting.text, "* OK") && !strings.HasPrefix(greeting.text, "* PREAUTH") {
		return nil, "", fmt.Errorf("imap: greeting: %s", strings.TrimSpace(greeting.text))
	}

	if _, err := s.do("LOGIN " + quoteIMAP(c.username) + " " + quoteIMAP(pw)); err != nil {
		return nil, "", err
	}

	selectLines, err := s.do("SELECT INBOX")
	if err != nil {
		return nil, "", err
	}

	var validity, uidNext uint64
	var haveValidity, haveNext bool
	for _, ln := range selectLines {
		if v, ok := fieldUint(ln.text, "[UIDVALIDITY"); ok {
			validity, haveValidity = v, true
		}
		if v, ok := fieldUint(ln.text, "[UIDNEXT"); ok {
			uidNext, haveNext = v, true
		}
	}
	if !haveValidity {
		return nil, "", fmt.Errorf("imap: select: response missing UIDVALIDITY")
	}
	if !haveNext {
		return nil, "", fmt.Errorf("imap: select: response missing UIDNEXT")
	}

	// First poll ever, or a mailbox reset (UIDVALIDITY changed): anchor to the
	// current state and return no messages.
	storedValidity, storedNext, ok := parseCursor(cursor)
	if !ok || storedValidity != validity {
		return nil, formatCursor(validity, uidNext), nil
	}

	// Nothing new: the cursor already sits at or past the current UIDNEXT.
	if storedNext >= uidNext {
		return nil, formatCursor(validity, uidNext), nil
	}

	// Fetch everything from the last seen boundary forward. A UID range "n:*"
	// always includes the highest-UID message even when n is above it (RFC 3501),
	// so filter to UID >= storedNext to avoid re-returning the boundary message.
	fetchLines, err := s.do(fmt.Sprintf("UID FETCH %d:* (UID BODY.PEEK[])", storedNext))
	if err != nil {
		return nil, "", err
	}

	nextUID := uidNext
	for _, ln := range fetchLines {
		uid, raw, ok := parseFetch(ln)
		if !ok || uid < storedNext {
			continue
		}
		if uid+1 > nextUID {
			nextUID = uid + 1
		}
		msgs = append(msgs, Inbound{
			Raw:       raw,
			MessageID: bracket(messageID(raw)),
			Recipient: recipient(raw),
		})
	}

	return msgs, formatCursor(validity, nextUID), nil
}

// --- protocol session ---------------------------------------------------------

// imapSession is one connection's command/response loop, tagging each command.
type imapSession struct {
	conn net.Conn
	r    *bufio.Reader
	seq  int
}

// do writes one tagged command and reads until its tagged completion, returning
// the untagged lines it collected along the way. A non-OK completion is an error;
// the command verb is named but its arguments are not, so a failed LOGIN never
// leaks the password.
func (s *imapSession) do(command string) (untagged []line, err error) {
	s.seq++
	tag := "a" + strconv.Itoa(s.seq)
	if _, err := s.conn.Write([]byte(tag + " " + command + "\r\n")); err != nil {
		return nil, fmt.Errorf("imap: write %s: %w", verb(command), err)
	}
	for {
		ln, err := readLine(s.r)
		if err != nil {
			return nil, fmt.Errorf("imap: read %s: %w", verb(command), err)
		}
		if strings.HasPrefix(ln.text, tag+" ") {
			status := strings.TrimSpace(ln.text[len(tag)+1:])
			if !strings.HasPrefix(status, "OK") {
				return untagged, fmt.Errorf("imap: %s: %s", verb(command), status)
			}
			return untagged, nil
		}
		untagged = append(untagged, ln)
	}
}

// verb is the first word of a command, safe to name in an error (the rest of a
// LOGIN command carries the password).
func verb(command string) string {
	if i := strings.IndexByte(command, ' '); i >= 0 {
		return command[:i]
	}
	return command
}

// --- line reading -------------------------------------------------------------

// line is one logical IMAP response line. IMAP may splice binary literals into a
// line (BODY[] arrives as a literal), so text holds the line with each literal
// removed and literals holds those byte runs in order.
type line struct {
	text     string
	literals [][]byte
}

// readLine reads one logical line, resolving IMAP literals. A literal is signalled
// by a trailing "{n}" before CRLF; the next n bytes are the literal data and the
// line then continues, so reading loops until it ends on a non-literal segment.
func readLine(r *bufio.Reader) (line, error) {
	var text strings.Builder
	var lits [][]byte
	for {
		seg, err := r.ReadString('\n')
		if err != nil {
			return line{}, err
		}
		seg = strings.TrimRight(seg, "\r\n")
		if n, ok := literalSize(seg); ok {
			text.WriteString(seg[:strings.LastIndexByte(seg, '{')])
			buf := make([]byte, n)
			if _, err := io.ReadFull(r, buf); err != nil {
				return line{}, err
			}
			lits = append(lits, buf)
			continue
		}
		text.WriteString(seg)
		break
	}
	return line{text: text.String(), literals: lits}, nil
}

// literalSize reports the byte count of a trailing "{n}" literal marker.
func literalSize(seg string) (int, bool) {
	if !strings.HasSuffix(seg, "}") {
		return 0, false
	}
	open := strings.LastIndexByte(seg, '{')
	if open < 0 {
		return 0, false
	}
	inner := strings.TrimSuffix(seg[open+1:len(seg)-1], "+")
	n, err := strconv.Atoi(inner)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// --- parsing ------------------------------------------------------------------

// parseFetch pulls the UID and raw message bytes out of an untagged FETCH line.
func parseFetch(ln line) (uid uint64, raw []byte, ok bool) {
	if !strings.Contains(ln.text, " FETCH ") {
		return 0, nil, false
	}
	uid, ok = fieldUint(ln.text, "UID")
	if !ok || len(ln.literals) == 0 {
		return 0, nil, false
	}
	return uid, ln.literals[0], true
}

// fieldUint finds "<key> <digits>" in text and returns the number. It doubles for
// the bracketed SELECT codes by passing key "[UIDVALIDITY" or "[UIDNEXT".
func fieldUint(text, key string) (uint64, bool) {
	i := strings.Index(text, key+" ")
	if i < 0 {
		return 0, false
	}
	j := i + len(key) + 1
	k := j
	for k < len(text) && text[k] >= '0' && text[k] <= '9' {
		k++
	}
	if k == j {
		return 0, false
	}
	n, err := strconv.ParseUint(text[j:k], 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// messageID reads the Message-ID header out of raw RFC 5322 bytes, keeping just
// the angle-bracketed id.
func messageID(raw []byte) string {
	v := headerValue(raw, "Message-ID")
	if a := strings.IndexByte(v, '<'); a >= 0 {
		if b := strings.IndexByte(v[a:], '>'); b >= 0 {
			return v[a : a+b+1]
		}
	}
	return strings.TrimSpace(v)
}

// recipient reads the first To address out of raw RFC 5322 bytes — the envelope
// recipient the router keys on.
func recipient(raw []byte) string {
	return firstAddress(headerValue(raw, "To"))
}

// headerValue returns the first header named name (case-insensitive), unfolding
// continuation lines.
func headerValue(raw []byte, name string) string {
	head := string(raw)
	if i := strings.Index(head, "\r\n\r\n"); i >= 0 {
		head = head[:i]
	} else if i := strings.Index(head, "\n\n"); i >= 0 {
		head = head[:i]
	}
	target := strings.ToLower(name) + ":"
	lines := strings.Split(head, "\n")
	for i := 0; i < len(lines); i++ {
		ln := strings.TrimRight(lines[i], "\r")
		if !strings.HasPrefix(strings.ToLower(ln), target) {
			continue
		}
		val := strings.TrimSpace(ln[len(target):])
		for i+1 < len(lines) {
			nx := strings.TrimRight(lines[i+1], "\r")
			if nx == "" || (nx[0] != ' ' && nx[0] != '\t') {
				break
			}
			val += " " + strings.TrimSpace(nx)
			i++
		}
		return val
	}
	return ""
}

// firstAddress extracts the first email address from a header address list,
// preferring the angle-bracketed form so a quoted display name with a comma does
// not split the wrong way.
func firstAddress(list string) string {
	list = strings.TrimSpace(list)
	if a := strings.IndexByte(list, '<'); a >= 0 {
		if b := strings.IndexByte(list[a:], '>'); b >= 0 {
			return strings.TrimSpace(list[a+1 : a+b])
		}
	}
	if i := strings.IndexByte(list, ','); i >= 0 {
		list = list[:i]
	}
	return strings.TrimSpace(list)
}

// --- cursor -------------------------------------------------------------------

// formatCursor renders the opaque poll cursor: "<uidvalidity>/<uidnext>".
func formatCursor(validity, uidNext uint64) string {
	return strconv.FormatUint(validity, 10) + "/" + strconv.FormatUint(uidNext, 10)
}

// parseCursor reads a cursor back; ok is false for the empty or malformed cursor,
// which the caller treats as "anchor to now".
func parseCursor(s string) (validity, uidNext uint64, ok bool) {
	a, b, found := strings.Cut(s, "/")
	if !found {
		return 0, 0, false
	}
	va, err1 := strconv.ParseUint(strings.TrimSpace(a), 10, 64)
	vb, err2 := strconv.ParseUint(strings.TrimSpace(b), 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return va, vb, true
}

// --- quoting ------------------------------------------------------------------

// quoteIMAP wraps s as an IMAP quoted string, escaping backslash and quote.
func quoteIMAP(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
