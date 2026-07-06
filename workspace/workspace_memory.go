package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/dhamidi/k-si/mime"
)

// Memory is the in-memory twin of the OS workspace (docs/05): a tree per task,
// mutex-guarded, with all state in the struct so the runner can keep one
// instance across a simulated crash/restart — the way a real directory would
// survive (docs/13). Nothing is global; there is no time.Now and no randomness,
// so listings and hashes are deterministic. It is shared with the sim harness,
// which writes out/ and transcripts through WriteOut/WriteTranscript.
type Memory struct {
	mu    sync.Mutex
	trees map[int64]*tree
}

// tree is one task-<id>/ directory: named files under in/ and out/, plus a
// transcript per run.
type tree struct {
	in          map[string]file  // filename -> file, the in/ box
	out         map[string]file  // filename -> file, the out/ box
	transcripts map[int64][]byte // runID -> transcript bytes
}

// file is a stored file's bytes plus the per-part metadata carried across the
// seam. ContentType is normalised to a non-empty value at write time.
type file struct {
	contentType string
	bytes       []byte
}

var _ Workspace = (*Memory)(nil)

// NewMemory builds the empty sim workspace shared with the sim harness.
func NewMemory() *Memory {
	return &Memory{trees: make(map[int64]*tree)}
}

// Root reports the synthetic location backing the in-memory tree.
func (m *Memory) Root() string { return "memory://workspace" }

// Create makes task-<id>/ with empty in/ and out/. Idempotent: an existing
// task is left untouched.
func (m *Memory) Create(taskID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensure(taskID)
	return nil
}

// Exists reports whether task-<id>/ has been created.
func (m *Memory) Exists(taskID int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.trees[taskID]
	return ok
}

// LayIn writes parts into in/, overwriting a same-named file and appending new
// ones across turns. The task tree is created on demand.
func (m *Memory) LayIn(taskID int64, parts []mime.Part) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.ensure(taskID)
	return writeInto("in", t.in, parts)
}

// WriteOut writes parts into out/ (the sim harness depositing a turn's output),
// with the same overwrite/append semantics as LayIn.
func (m *Memory) WriteOut(taskID int64, parts []mime.Part) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.ensure(taskID)
	return writeInto("out", t.out, parts)
}

// Harvest reads out/ into parts, reply.txt first (if present) and the rest in
// path order, each Filename the path relative to out/ (a nested part keeps its
// "skills/pay/SKILL.md" path; a flat one stays a plain name).
func (m *Memory) Harvest(taskID int64) ([]mime.Part, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.trees[taskID]
	if !ok {
		return nil, fmt.Errorf("workspace: task %d: no workspace", taskID)
	}

	names := sortedKeys(t.out)
	parts := make([]mime.Part, 0, len(names))
	if _, ok := t.out["reply.txt"]; ok {
		parts = append(parts, partOf("reply.txt", t.out["reply.txt"]))
	}
	for _, name := range names {
		if name == "reply.txt" {
			continue
		}
		parts = append(parts, partOf(name, t.out[name]))
	}
	return parts, nil
}

// WriteTranscript stores run-<runID>'s transcript bytes verbatim (a copy).
func (m *Memory) WriteTranscript(taskID, runID int64, b []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.ensure(taskID)
	t.transcripts[runID] = append([]byte(nil), b...)
	return nil
}

// ReadTranscript returns a copy of a run's transcript bytes.
func (m *Memory) ReadTranscript(taskID, runID int64) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.trees[taskID]
	if !ok {
		return nil, fmt.Errorf("workspace: task %d: no workspace", taskID)
	}
	b, ok := t.transcripts[runID]
	if !ok {
		return nil, fmt.Errorf("workspace: task %d run %d: no transcript", taskID, runID)
	}
	return append([]byte(nil), b...), nil
}

// Files lists every file under task-<id>/ in a deterministic order — in/
// (sorted), then out/ (sorted), then each transcript (by run id) — with
// Filename set to the path relative to task-<id> so archival can classify each
// one (see the Workspace doc).
func (m *Memory) Files(taskID int64) ([]mime.Part, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.trees[taskID]
	if !ok {
		return nil, fmt.Errorf("workspace: task %d: no workspace", taskID)
	}
	return m.filesLocked(t), nil
}

// Delete removes task-<id>/, refusing unless every current file's hex sha256 is
// present in archived (docs/05, docs/13 archive-before-delete). Deleting an
// absent task is a no-op so cleanup is idempotent after a crash.
func (m *Memory) Delete(taskID int64, archived map[string]bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.trees[taskID]
	if !ok {
		return nil
	}
	for _, p := range m.filesLocked(t) {
		sum := sha256.Sum256(p.Bytes)
		if !archived[hex.EncodeToString(sum[:])] {
			return fmt.Errorf("workspace: task %d: refusing to delete, %q not archived", taskID, p.Filename)
		}
	}
	delete(m.trees, taskID)
	return nil
}

// filesLocked builds the relative-path Part listing; callers hold m.mu.
func (m *Memory) filesLocked(t *tree) []mime.Part {
	var parts []mime.Part
	for _, name := range sortedKeys(t.in) {
		parts = append(parts, partOf("in/"+name, t.in[name]))
	}
	for _, name := range sortedKeys(t.out) {
		parts = append(parts, partOf("out/"+name, t.out[name]))
	}
	for _, runID := range sortedRunIDs(t.transcripts) {
		parts = append(parts, mime.Part{
			Filename:    fmt.Sprintf("transcript-%d.jsonl", runID),
			ContentType: "application/jsonl",
			Bytes:       append([]byte(nil), t.transcripts[runID]...),
		})
	}
	return parts
}

// ensure returns task-<id>/'s tree, creating it if absent; callers hold m.mu.
func (m *Memory) ensure(taskID int64) *tree {
	t, ok := m.trees[taskID]
	if !ok {
		t = &tree{
			in:          make(map[string]file),
			out:         make(map[string]file),
			transcripts: make(map[int64][]byte),
		}
		m.trees[taskID] = t
	}
	return t
}

// writeInto copies parts into a box keyed by the part's relative path,
// overwriting by path and normalising an empty ContentType from the extension.
// Each path is validated to stay inside the box (no absolute, no "..") — the
// on-disk twin's os.MkdirAll tree is a flat map of relative paths here, but the
// same sandbox rule holds (decision-011).
func writeInto(boxName string, box map[string]file, parts []mime.Part) error {
	for _, p := range parts {
		rel, err := validBoxPath(boxName, p.Filename)
		if err != nil {
			return err
		}
		ct := p.ContentType
		if ct == "" {
			ct = defaultContentType(rel)
		}
		box[rel] = file{
			contentType: ct,
			bytes:       append([]byte(nil), p.Bytes...),
		}
	}
	return nil
}

// partOf rebuilds a mime.Part for a stored file under the given (possibly
// relative) name, copying the bytes so callers can't mutate the tree.
func partOf(name string, f file) mime.Part {
	return mime.Part{
		Filename:    name,
		ContentType: f.contentType,
		Bytes:       append([]byte(nil), f.bytes...),
	}
}

// defaultContentType picks a ContentType when a written Part left it empty:
// text/plain for .txt, application/octet-stream otherwise.
func defaultContentType(filename string) string {
	if strings.HasSuffix(filename, ".txt") {
		return "text/plain; charset=utf-8"
	}
	return "application/octet-stream"
}

// sortedKeys returns a box's filenames in lexical order for deterministic
// listings.
func sortedKeys(box map[string]file) []string {
	keys := make([]string, 0, len(box))
	for k := range box {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedRunIDs returns transcript run ids in ascending order.
func sortedRunIDs(m map[int64][]byte) []int64 {
	ids := make([]int64, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
