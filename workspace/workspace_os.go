package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dhamidi/k-si/mime"
)

// OS is the on-disk twin of Memory: a $WORKDIR the harness and the main agent
// share on a real filesystem (docs/05). Same layout and conventions as Memory —
// plain filenames in in/ and out/, transcript-<run>.jsonl per run, and Files
// yielding paths relative to task-<id> so the archive step can classify them.
type OS struct {
	root string
}

var _ Workspace = (*OS)(nil)

// NewOS builds the on-disk workspace rooted at $WORKDIR.
func NewOS(root string) *OS { return &OS{root: root} }

// Root reports the $WORKDIR backing this workspace.
func (o *OS) Root() string { return o.root }

// taskDir is $WORKDIR/task-<id>.
func (o *OS) taskDir(taskID int64) string {
	return filepath.Join(o.root, "task-"+strconv.FormatInt(taskID, 10))
}

// Exists reports whether task-<id>/ is present on disk.
func (o *OS) Exists(taskID int64) bool {
	info, err := os.Stat(o.taskDir(taskID))
	return err == nil && info.IsDir()
}

// Create makes task-<id>/ with in/ and out/. Idempotent.
func (o *OS) Create(taskID int64) error {
	for _, box := range []string{"in", "out"} {
		if err := os.MkdirAll(filepath.Join(o.taskDir(taskID), box), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// LayIn writes parts into in/ (overwriting a same-named file, adding new ones).
func (o *OS) LayIn(taskID int64, parts []mime.Part) error {
	return o.writeBox(taskID, "in", parts)
}

// WriteOut writes parts into out/ — the (real) harness calls this indirectly by
// writing files itself; it is here for symmetry and the sim-parity path.
func (o *OS) WriteOut(taskID int64, parts []mime.Part) error {
	return o.writeBox(taskID, "out", parts)
}

// WriteSkills provisions skill trees into task-<id>/.claude/skills/ (Flow D,
// decision-009), the layout the harness expects at ./skills/<name>/.
func (o *OS) WriteSkills(taskID int64, parts []mime.Part) error {
	return o.writeBox(taskID, SkillsBox, parts)
}

func (o *OS) writeBox(taskID int64, box string, parts []mime.Part) error {
	dir := filepath.Join(o.taskDir(taskID), box)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, p := range parts {
		// Each part is written at its relative path under the box (creating
		// intermediate dirs), validated to stay inside the box — no absolute path,
		// no ".." escape (decision-011).
		rel, err := validBoxPath(box, p.Filename)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, p.Bytes, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Harvest reads out/ into parts RECURSIVELY, Filename set to the path relative
// to out/ ("reply.txt", "skills/pay/SKILL.md"). reply.txt (top-level) is first,
// the rest path-sorted. A flat out/ yields plain names exactly as before.
func (o *OS) Harvest(taskID int64) ([]mime.Part, error) {
	dir := filepath.Join(o.taskDir(taskID), "out")
	var names []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	sort.Slice(names, func(i, j int) bool {
		if names[i] == "reply.txt" != (names[j] == "reply.txt") {
			return names[i] == "reply.txt"
		}
		return names[i] < names[j]
	})

	var parts []mime.Part
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			return nil, err
		}
		parts = append(parts, mime.Part{Filename: name, ContentType: contentType(name), Bytes: b})
	}
	return parts, nil
}

// WriteTranscript writes a run's transcript as task-<id>/transcript-<run>.jsonl.
func (o *OS) WriteTranscript(taskID, runID int64, b []byte) error {
	if err := os.MkdirAll(o.taskDir(taskID), 0o755); err != nil {
		return err
	}
	return os.WriteFile(o.transcriptPath(taskID, runID), b, 0o644)
}

// ReadTranscript reads a run's captured transcript.
func (o *OS) ReadTranscript(taskID, runID int64) ([]byte, error) {
	return os.ReadFile(o.transcriptPath(taskID, runID))
}

func (o *OS) transcriptPath(taskID, runID int64) string {
	return filepath.Join(o.taskDir(taskID), fmt.Sprintf("transcript-%d.jsonl", runID))
}

// Files lists every file under task-<id>/ with its path relative to that
// directory (in/…, out/…, transcript-…), deterministically ordered — the input
// to archival (docs/05).
func (o *OS) Files(taskID int64) ([]mime.Part, error) {
	root := o.taskDir(taskID)
	var parts []mime.Part

	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Skip a symlink entry — the persistent store is symlinked in at ./store
		// (Flow F, decision-012). Archival must never follow it (that would read,
		// archive, and — via Delete's archived-check — let it block deletion of
		// another task's live data). WalkDir does not descend into a symlinked
		// directory, so returning nil here elides the link entirely.
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		parts = append(parts, mime.Part{
			Filename:    filepath.ToSlash(rel),
			ContentType: contentType(rel),
			Bytes:       b,
		})
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	sort.Slice(parts, func(i, j int) bool { return parts[i].Filename < parts[j].Filename })
	return parts, nil
}

// Delete removes task-<id>/, but only once every live file's content hash is
// present in archived — the archive-before-delete invariant (docs/05, docs/13),
// enforced on real disk exactly as the sim twin enforces it.
func (o *OS) Delete(taskID int64, archived map[string]bool) error {
	files, err := o.Files(taskID)
	if err != nil {
		return err
	}
	for _, f := range files {
		sum := sha256.Sum256(f.Bytes)
		if !archived[hex.EncodeToString(sum[:])] {
			return fmt.Errorf("workspace: refusing to delete task-%d: %q is not archived", taskID, f.Filename)
		}
	}
	return os.RemoveAll(o.taskDir(taskID))
}

// contentType guesses a part's media type from its name, matching the sim twin.
func contentType(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".pdf":
		return "application/pdf"
	case ".jsonl", ".json":
		return "application/jsonl"
	default:
		return "application/octet-stream"
	}
}
