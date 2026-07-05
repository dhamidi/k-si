package mime

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
)

// Part is one file / MIME part — the unit that flows through in/, out/, inbox,
// outbox and archive (docs/02). A text body and a PDF attachment are both Parts.
type Part struct {
	Filename    string               // "invoice.pdf", "body.txt", "reply.txt"
	ContentType string               // "application/pdf", "text/plain; charset=utf-8"
	Header      textproto.MIMEHeader // optional per-part headers as seen on the wire
	Bytes       []byte               // decoded content (transfer-encoding removed)
}

// Message is a parsed inbound or assembled outbound email (docs/02). Header
// carries From/To/Cc/Subject/Message-ID/In-Reply-To/References and any X-Kasi-*
// metadata; Text is the text/plain body; Parts are the non-text attachments;
// Raw is the original (inbound) or assembled (outbound) RFC 5322 bytes.
type Message struct {
	Header mail.Header
	Text   string
	Parts  []Part
	Raw    []byte
}

// Parse reads RFC 5322 bytes into a Message, handling both single-part
// text/plain and multipart/mixed (a text body plus attachments). Transfer
// encodings (quoted-printable, base64) are decoded. The full header set is
// preserved on Message.Header, so Cc, In-Reply-To, References, Message-ID and
// X-Kasi-* are read via the usual mail.Header accessors (docs/02).
func Parse(raw []byte) (Message, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return Message{}, fmt.Errorf("mime: read message: %w", err)
	}

	out := Message{
		Header: msg.Header,
		Raw:    append([]byte(nil), raw...),
	}

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mediaType == "" {
		mediaType = "text/plain" // absent/invalid Content-Type: treat as plain text
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		if err := out.readMultipart(msg.Body, params["boundary"]); err != nil {
			return Message{}, err
		}
		return out, nil
	}

	body, err := io.ReadAll(msg.Body)
	if err != nil {
		return Message{}, fmt.Errorf("mime: read body: %w", err)
	}
	body, err = decodeTransfer(msg.Header.Get("Content-Transfer-Encoding"), body)
	if err != nil {
		return Message{}, err
	}
	out.Text = string(body)
	return out, nil
}

// readMultipart walks a multipart/mixed body: the first text/plain part without
// a filename becomes Text, everything else becomes an attachment Part.
func (out *Message) readMultipart(body io.Reader, boundary string) error {
	mr := multipart.NewReader(body, boundary)
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("mime: next part: %w", err)
		}

		content, err := io.ReadAll(p)
		if err != nil {
			return fmt.Errorf("mime: read part: %w", err)
		}
		// multipart transparently decodes quoted-printable and drops the header;
		// base64 is left for us to decode.
		if strings.EqualFold(strings.TrimSpace(p.Header.Get("Content-Transfer-Encoding")), "base64") {
			content, err = decodeBase64(content)
			if err != nil {
				return err
			}
		}

		filename := p.FileName()
		ptype, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		if out.Text == "" && filename == "" && (ptype == "" || ptype == "text/plain") {
			out.Text = string(content)
			continue
		}
		out.Parts = append(out.Parts, Part{
			Filename:    filename,
			ContentType: p.Header.Get("Content-Type"),
			Header:      p.Header,
			Bytes:       content,
		})
	}
}

// decodeTransfer reverses a Content-Transfer-Encoding on a single-part body.
func decodeTransfer(encoding string, raw []byte) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		return decodeBase64(raw)
	case "quoted-printable":
		b, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(raw)))
		if err != nil {
			return nil, fmt.Errorf("mime: quoted-printable decode: %w", err)
		}
		return b, nil
	default: // 7bit, 8bit, binary, empty
		return raw, nil
	}
}

// decodeBase64 decodes base64 that may carry the line breaks the wire format
// inserts every 76 characters.
func decodeBase64(raw []byte) ([]byte, error) {
	cleaned := strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, string(raw))
	b, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return nil, fmt.Errorf("mime: base64 decode: %w", err)
	}
	return b, nil
}
