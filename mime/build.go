package mime

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"sort"
)

// reservedHeaders are set by Build itself; any caller-supplied copy is dropped
// so the assembled bytes have exactly one of each.
var reservedHeaders = map[string]bool{
	"Content-Type":              true,
	"Content-Transfer-Encoding": true,
	"Mime-Version":              true,
}

// Build assembles RFC 5322 bytes from headers, a text body and attachment parts:
// multipart/mixed when parts are present, otherwise text/plain (docs/02). Output
// is deterministic — headers are emitted in sorted order and the multipart
// boundary is derived from the Message-ID (see boundaryFor), never random or
// time-based — so replay and scenario tests see identical bytes (docs/13).
func Build(hdr map[string][]string, text string, parts []Part) ([]byte, error) {
	var body bytes.Buffer
	var contentType, cte string

	if len(parts) == 0 {
		contentType = "text/plain; charset=utf-8"
		cte = "quoted-printable"
		if err := writeQuotedPrintable(&body, []byte(text)); err != nil {
			return nil, err
		}
	} else {
		boundary := boundaryFor(headerValue(hdr, "Message-Id"))
		mw := multipart.NewWriter(&body)
		if err := mw.SetBoundary(boundary); err != nil {
			return nil, fmt.Errorf("mime: set boundary: %w", err)
		}
		contentType = "multipart/mixed; boundary=" + boundary

		th := textproto.MIMEHeader{}
		th.Set("Content-Type", "text/plain; charset=utf-8")
		th.Set("Content-Transfer-Encoding", "quoted-printable")
		tw, err := mw.CreatePart(th)
		if err != nil {
			return nil, fmt.Errorf("mime: create text part: %w", err)
		}
		if err := writeQuotedPrintable(tw, []byte(text)); err != nil {
			return nil, err
		}

		for _, p := range parts {
			if err := writeAttachment(mw, p); err != nil {
				return nil, err
			}
		}
		if err := mw.Close(); err != nil {
			return nil, fmt.Errorf("mime: close multipart: %w", err)
		}
	}

	var out bytes.Buffer
	writeHeaders(&out, hdr)
	out.WriteString("MIME-Version: 1.0\r\n")
	out.WriteString("Content-Type: " + contentType + "\r\n")
	if cte != "" {
		out.WriteString("Content-Transfer-Encoding: " + cte + "\r\n")
	}
	out.WriteString("\r\n")
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

// writeAttachment writes one binary part as base64 with a filename disposition.
func writeAttachment(mw *multipart.Writer, p Part) error {
	h := textproto.MIMEHeader{}
	ct := p.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	h.Set("Content-Type", ct)
	h.Set("Content-Transfer-Encoding", "base64")
	if p.Filename != "" {
		h.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", p.Filename))
	}

	w, err := mw.CreatePart(h)
	if err != nil {
		return fmt.Errorf("mime: create attachment part: %w", err)
	}
	return writeBase64(w, p.Bytes)
}

// writeHeaders emits the caller's headers (minus the reserved ones) in a stable
// order: canonical keys sorted, values kept in the caller's order.
func writeHeaders(out *bytes.Buffer, hdr map[string][]string) {
	canonical := make(map[string][]string, len(hdr))
	keys := make([]string, 0, len(hdr))
	for k, vs := range hdr {
		ck := textproto.CanonicalMIMEHeaderKey(k)
		if reservedHeaders[ck] {
			continue
		}
		if _, seen := canonical[ck]; !seen {
			keys = append(keys, ck)
		}
		canonical[ck] = append(canonical[ck], vs...)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range canonical[k] {
			out.WriteString(k)
			out.WriteString(": ")
			out.WriteString(v)
			out.WriteString("\r\n")
		}
	}
}

// boundaryFor derives a fixed multipart boundary from the Message-ID so the same
// message always assembles to the same bytes. An empty seed still yields a valid,
// stable boundary.
func boundaryFor(messageID string) string {
	sum := sha256.Sum256([]byte(messageID))
	return "kasi-boundary-" + hex.EncodeToString(sum[:16])
}

// headerValue returns the first value for a header key, matching case-insensitively.
func headerValue(hdr map[string][]string, key string) string {
	ck := textproto.CanonicalMIMEHeaderKey(key)
	for k, vs := range hdr {
		if textproto.CanonicalMIMEHeaderKey(k) == ck && len(vs) > 0 {
			return vs[0]
		}
	}
	return ""
}

// writeQuotedPrintable encodes body with quoted-printable (deterministic).
func writeQuotedPrintable(out io.Writer, body []byte) error {
	qw := quotedprintable.NewWriter(out)
	if _, err := qw.Write(body); err != nil {
		return fmt.Errorf("mime: quoted-printable encode: %w", err)
	}
	if err := qw.Close(); err != nil {
		return fmt.Errorf("mime: quoted-printable close: %w", err)
	}
	return nil
}

// writeBase64 encodes body as base64 folded at 76 columns (RFC 2045).
func writeBase64(out io.Writer, body []byte) error {
	encoded := base64.StdEncoding.EncodeToString(body)
	const width = 76
	for i := 0; i < len(encoded); i += width {
		end := i + width
		if end > len(encoded) {
			end = len(encoded)
		}
		if _, err := out.Write([]byte(encoded[i:end] + "\r\n")); err != nil {
			return fmt.Errorf("mime: base64 write: %w", err)
		}
	}
	return nil
}
