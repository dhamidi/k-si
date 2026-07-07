package mime

import "strings"

// LayIn maps a parsed Message to the files written into a task workspace's in/
// directory (docs/02, docs/05): body.txt opens with a small envelope header block
// (Subject/From/Date, whichever are present), then a blank line and the text body;
// each attachment becomes a file preserving its Filename and ContentType. The
// envelope is what lets a worker agent see the subject and sender it is acting on —
// reading the body alone used to hide them. The agent sees plain files, not MIME.
func LayIn(m Message) []Part {
	parts := make([]Part, 0, len(m.Parts)+1)
	parts = append(parts, Part{
		Filename:    "body.txt",
		ContentType: "text/plain; charset=utf-8",
		Bytes:       []byte(bodyWithEnvelope(m)),
	})
	for _, p := range m.Parts {
		parts = append(parts, Part{
			Filename:    p.Filename,
			ContentType: p.ContentType,
			Header:      p.Header,
			Bytes:       p.Bytes,
		})
	}
	return parts
}

// bodyWithEnvelope prepends an RFC-822-style header block (Subject, From, Date —
// only those actually present) to the text body, separated by one blank line.
// Subject passes through DecodeSubject (idempotent) defensively: Parse already
// decodes it, but this keeps a Message assembled without Parse legible too. A
// message carrying none of these headers yields the bare body unchanged, so
// header-less inputs behave exactly as before this envelope was added.
func bodyWithEnvelope(m Message) string {
	var b strings.Builder
	if subject := DecodeSubject(m.Header.Get("Subject")); subject != "" {
		b.WriteString("Subject: " + subject + "\n")
	}
	if from := m.Header.Get("From"); from != "" {
		b.WriteString("From: " + from + "\n")
	}
	if date := m.Header.Get("Date"); date != "" {
		b.WriteString("Date: " + date + "\n")
	}
	if b.Len() == 0 {
		return m.Text
	}
	b.WriteString("\n")
	b.WriteString(m.Text)
	return b.String()
}
