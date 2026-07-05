package mime

// LayIn maps a parsed Message to the files written into a task workspace's in/
// directory (docs/02, docs/05): body.txt holds the text body, and each
// attachment becomes a file preserving its Filename and ContentType. The agent
// sees plain files, not MIME.
func LayIn(m Message) []Part {
	parts := make([]Part, 0, len(m.Parts)+1)
	parts = append(parts, Part{
		Filename:    "body.txt",
		ContentType: "text/plain; charset=utf-8",
		Bytes:       []byte(m.Text),
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
