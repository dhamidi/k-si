// Package skilltree packs an Agent Skills directory tree into a tar blob and
// reads it back (decision-010). A skill is a directory — a required SKILL.md plus
// optional scripts/, references/, assets/, and any files — stored atomically as
// one tar in the store's `skill` table's single content BLOB, provisioned and
// versioned as a unit. Pure: only the mime object model and the standard library
// (archive/tar) are imported; nothing here touches the runtime.
package skilltree

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/dhamidi/k-si/mime"
)

// Pack writes the tree into a tar blob. Each part's Filename is its path relative
// to the skill root ("SKILL.md", "scripts/run.sh"); entries are written in sorted
// path order so the same tree packs byte-for-byte identically.
func Pack(parts []mime.Part) ([]byte, error) {
	ordered := append([]mime.Part(nil), parts...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Filename < ordered[j].Filename })

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, p := range ordered {
		hdr := &tar.Header{
			Name:     p.Filename,
			Mode:     0o644,
			Size:     int64(len(p.Bytes)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("skilltree: write header %q: %w", p.Filename, err)
		}
		if _, err := tw.Write(p.Bytes); err != nil {
			return nil, fmt.Errorf("skilltree: write body %q: %w", p.Filename, err)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("skilltree: close: %w", err)
	}
	return buf.Bytes(), nil
}

// List returns the tree's entry paths, sorted. Non-regular entries are skipped.
func List(tarBytes []byte) ([]string, error) {
	var names []string
	tr := tar.NewReader(bytes.NewReader(tarBytes))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("skilltree: list: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		names = append(names, hdr.Name)
	}
	sort.Strings(names)
	return names, nil
}

// Read returns one entry's bytes. The bool is false (no error) when the tar holds
// no entry at that path.
func Read(tarBytes []byte, path string) ([]byte, bool, error) {
	tr := tar.NewReader(bytes.NewReader(tarBytes))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("skilltree: read %q: %w", path, err)
		}
		if hdr.Name != path {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return nil, false, fmt.Errorf("skilltree: read %q: %w", path, err)
		}
		return b, true, nil
	}
}

// Unpack reads the whole tree back into parts, sorted by path, each Filename the
// path relative to the skill root. It is the inverse of Pack.
func Unpack(tarBytes []byte) ([]mime.Part, error) {
	var parts []mime.Part
	tr := tar.NewReader(bytes.NewReader(tarBytes))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("skilltree: unpack: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("skilltree: unpack %q: %w", hdr.Name, err)
		}
		parts = append(parts, mime.Part{Filename: hdr.Name, Bytes: b})
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].Filename < parts[j].Filename })
	return parts, nil
}

// Frontmatter is a MINIMAL YAML-frontmatter reader for a SKILL.md: if the content
// opens with a "---" line it scans to the closing "---" and pulls the single-line
// "name:" and "description:" values, trimming surrounding quotes and space. It
// tolerates absence — no frontmatter, or a missing key, yields "". No YAML
// dependency: only these two scalar fields are needed (decision-009).
func Frontmatter(skillMD []byte) (name, description string) {
	lines := strings.Split(string(skillMD), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return "", ""
	}
	for _, raw := range lines[1:] {
		line := strings.TrimRight(raw, "\r")
		if line == "---" {
			break
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "name":
			name = trimScalar(val)
		case "description":
			description = trimScalar(val)
		}
	}
	return name, description
}

// trimScalar strips surrounding whitespace and a single layer of matching quotes
// from a frontmatter scalar value.
func trimScalar(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = s[1 : len(s)-1]
		}
	}
	return s
}
