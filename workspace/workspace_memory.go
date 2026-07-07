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
	// failProvisioned scripts the next N ProvisionedMemory reads to fail — the
	// harvest's fault-injection knob, mirroring SimMail.failSend (docs/13). Only
	// capture-memory reads ProvisionedMemory, so failing it leaves that harvest
	// entirely un-emitted, which is the crash-mid-harvest a scenario needs to
	// exercise HarvestPending reconciliation. Like the tree, it survives a
	// simulated crash (the struct outlives the App), so a harvest that failed stays
	// failed until reconciliation retries it on restart.
	failProvisioned int
}

// tree is one task-<id>/ directory: named files under in/, out/, and skills/,
// plus a transcript per run.
type tree struct {
	in          map[string]file  // relative path -> file, the in/ box
	out         map[string]file  // relative path -> file, the out/ box
	skills      map[string]file  // relative path -> file, the skills/ box (Flow D)
	transcripts map[int64][]byte // runID -> transcript bytes
	// provisionedMemory is this run's pinned memory name set, recorded by
	// WriteMemory and read by ProvisionedMemory (feature-memory.md). It is
	// workspace-private: filesLocked never lists it, so archival ignores it. Like
	// the tree itself, it survives a simulated crash (the struct outlives the App).
	provisionedMemory []string
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

// WriteSkills provisions skill trees into the skills/ box (Flow D,
// decision-009), so a later turn finds ./skills/<name>/SKILL.md. Same
// overwrite/append-by-path semantics as WriteOut.
func (m *Memory) WriteSkills(taskID int64, parts []mime.Part) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.ensure(taskID)
	return writeInto(SkillsBox, t.skills, parts)
}

// WriteMemory provisions the memory collection into in/: each note at
// memory/<name>.md (raw Content) plus the MEMORY.md index (feature-memory.md),
// and records this run's provisioned name set workspace-private. Same
// overwrite/append-by-path semantics as LayIn for the in/ files; the manifest is
// held apart from the tree so Files never surfaces it.
func (m *Memory) WriteMemory(taskID int64, mems []MemoryFile) error {
	// Drop any box-unsafe name (defense in depth) so a single poisoned entry never
	// errors the whole provisioning; the manifest and the in/ files run off survivors.
	mems = provisionableMemories(mems)

	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.ensure(taskID)
	t.provisionedMemory = memoryNames(mems)

	// Prune stale memory/ notes so in/ exactly equals the CURRENT collection — on a
	// resume a memory forgotten since an earlier turn must not linger as
	// memory/<name>.md and be re-handed to the agent (Fix 4, feature-memory.md). Only
	// keys that already exist are removed, so a fresh, empty run touches nothing and
	// its in/ box stays byte-identical to before.
	keep := make(map[string]bool, len(mems))
	for _, mem := range mems {
		keep[mem.Name] = true
	}
	prefix := MemoryDir + "/"
	for key := range t.in {
		rel, ok := strings.CutPrefix(key, prefix)
		if !ok || strings.Contains(rel, "/") {
			continue
		}
		name, ok := strings.CutSuffix(rel, ".md")
		if !ok || keep[name] {
			continue
		}
		delete(t.in, key)
	}

	// An empty collection lays nothing — no notes, no index — so a run with no
	// memories has an unchanged in/ box (feature-memory.md: the index rides with the
	// notes it lists). Drop a stale index so a resume that forgot the last memory
	// leaves no orphaned MEMORY.md; the provisioned set stays recorded (empty).
	if len(mems) == 0 {
		delete(t.in, MemoryIndexName)
		return nil
	}
	return writeInto("in", t.in, memoryParts(mems))
}

// ProvisionedMemory returns this run's pinned provisioned memory names — a copy,
// so callers can't mutate the tree (feature-memory.md).
func (m *Memory) ProvisionedMemory(taskID int64) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failProvisioned > 0 {
		m.failProvisioned--
		return nil, fmt.Errorf("workspace: simulated provisioned-memory failure")
	}
	t, ok := m.trees[taskID]
	if !ok {
		return nil, nil
	}
	// A non-nil empty slice after an empty provision, matching the OS twin (whose
	// manifest unmarshals `[]` into a non-nil empty slice) so the twins are
	// byte-identical in behaviour (Fix 5, the twin rule): append to a nil base would
	// yield nil when the set is empty.
	names := make([]string, len(t.provisionedMemory))
	copy(names, t.provisionedMemory)
	return names, nil
}

// FailNext scripts the next n operations on an op to fail (docs/13). Only
// "harvest" is meaningful today: it fails the ProvisionedMemory read the
// capture-memory harvest depends on, so a scenario can drive a crash mid-harvest
// and prove HarvestPending reconciliation recovers it. This is a sim-only test
// hook (not on the Workspace interface), mirroring SimMail.FailNext.
func (m *Memory) FailNext(op string, n int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if op == "harvest" {
		m.failProvisioned += n
	}
}

// DeleteIn removes a file from the in/ box by its box-relative path — an agent
// forgetting a memory (feature-memory.md). Absent file is a no-op; the path is
// validated to stay inside the box (decision-011).
func (m *Memory) DeleteIn(taskID int64, rel string) error {
	name, err := validBoxPath("in", rel)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.trees[taskID]
	if !ok {
		return nil
	}
	delete(t.in, name)
	return nil
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
	for _, name := range sortedKeys(t.skills) {
		parts = append(parts, partOf(SkillsBox+"/"+name, t.skills[name]))
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
			skills:      make(map[string]file),
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
