// Package skilltree packs an Agent Skills directory tree into a tar blob and
// reads it back (decision-010). A skill is a directory — a required SKILL.md plus
// optional scripts/, references/, assets/, and any files — stored atomically as
// one tar in the store's `skill` table's single content BLOB, provisioned and
// versioned as a unit. Pure: the mime object model, the standard library
// (archive/tar), and a YAML parser for the SKILL.md frontmatter are imported;
// nothing here touches the runtime.
package skilltree

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"go.yaml.in/yaml/v4"

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

// Frontmatter reads a SKILL.md's YAML frontmatter and returns its "name" and
// "description". If the content opens with a "---" line, the block up to the
// closing "---" is parsed as YAML — so block scalars (">-", "|"), quoted and
// multi-line values are all handled, not just single-line "key: value". It
// tolerates absence and malformation: no frontmatter, no closing fence, a missing
// key, or unparseable YAML each yield "". The description is space-trimmed so a
// folded scalar's trailing newline doesn't reach the UI.
func Frontmatter(skillMD []byte) (name, description string) {
	block, ok := frontmatterBlock(skillMD)
	if !ok {
		return "", ""
	}
	var fm struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(block, &fm); err != nil {
		return "", ""
	}
	return strings.TrimSpace(fm.Name), strings.TrimSpace(fm.Description)
}

// frontmatterBlock returns the bytes between the opening and closing "---" fences.
// ok is false if the content does not open with a "---" line or has no closing
// fence — either way there is no frontmatter to parse.
func frontmatterBlock(md []byte) (block []byte, ok bool) {
	lines := strings.Split(string(md), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return nil, false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			return []byte(strings.Join(lines[1:i], "\n")), true
		}
	}
	return nil, false
}
