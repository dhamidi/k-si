// Package workspace is the per-task scratch-directory edge (docs/05): the
// filesystem seam where the main agent lays inbound mail into a task's in/,
// runs a harness, and harvests out/ into a reply. Domains hold a Workspace in
// their Edges. Two twins implement it — Memory (the simulation ring, an
// in-memory tree that SURVIVES a simulated crash the way a real directory
// would) and OS ($WORKDIR on disk, stage-2 wiring). Only the mime object model
// (docs/02) and the standard library are imported; nothing here touches the
// runtime.
package workspace

import (
	"encoding/json"
	"fmt"
	"log"
	"path"
	"path/filepath"
	"strings"

	"github.com/dhamidi/k-si/mime"
)

// MemoryFile is one memory to provision, as a workspace-LOCAL value — so this edge
// stays free of the memory domain and imports only the mime object model and the
// standard library (the interface doc). The agents effect maps memory.Memory onto
// it: Name is the note's slug, Content its raw bytes, Description the already-derived
// one-line entry the MEMORY.md index renders (the edge never parses frontmatter).
type MemoryFile struct {
	Name        string
	Content     []byte
	Description string
}

// AppFile is one registered app to advertise to a run, as a workspace-LOCAL
// value — so this edge stays free of the apps domain and imports only the mime
// object model and the standard library. The agents effect maps apps.App onto
// it: Name is the app's slug, URL the localhost origin the agent calls it on,
// and Operations the app's RAW app.json bytes (the edge extracts the operations
// array, never trusting a pre-parsed value — store-raw/derive-on-replay).
type AppFile struct {
	Name       string
	URL        string
	Operations []byte
}

// AppsIndexName is the single in/ file listing the apps a run may call
// (feature-apps.md): a JSON object keyed by app name, each entry its localhost
// URL and operations. Written fresh every run (WriteApps), so it never lists a
// removed app.
const AppsIndexName = "apps.json"

// appsJSON renders the in/apps.json body from the app set — the twin-shared
// renderer both the OS and memory workspaces write verbatim, so the file a run
// sees is identical across the real edge and its sim twin (docs/12). Keys sort
// (encoding/json sorts map keys), so the bytes are deterministic.
func appsJSON(apps []AppFile) []byte {
	type entry struct {
		URL        string          `json:"url"`
		Operations json.RawMessage `json:"operations"`
	}
	out := make(map[string]entry, len(apps))
	for _, a := range apps {
		out[a.Name] = entry{URL: a.URL, Operations: appOperations(a.Operations)}
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return []byte("{}\n")
	}
	return append(b, '\n')
}

// appOperations pulls the "operations" array out of an app's raw app.json,
// defaulting to an empty array when the file is absent, unparseable, or has no
// operations — a pure-UI app that just leaves it out still gets a well-formed
// entry (feature-apps.md).
func appOperations(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("[]")
	}
	var doc struct {
		Operations json.RawMessage `json:"operations"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc.Operations) == 0 {
		return json.RawMessage("[]")
	}
	return doc.Operations
}

// Workspace is the filesystem edge for a task's ephemeral scratch directory,
// laid out as $WORKDIR/task-<id>/{in,out}/ plus per-run transcripts (docs/05).
//
// Filename conventions on the mime.Part values that cross this seam:
//
//   - LayIn and WriteOut take Parts whose Filename is a path RELATIVE to the box
//     ("body.txt", "reply.txt", "skills/pay/SKILL.md"); each lands at that
//     relative path under in/ or out/, intermediate directories created. A flat
//     name is a depth-1 path, so flat behaviour is unchanged. Paths are
//     validated to stay within the box — an absolute path or a ".." segment is
//     rejected (decision-011: the workspace is a sandbox boundary).
//   - Harvest returns Parts for out/'s whole tree with Filename the path
//     RELATIVE to out/ ("reply.txt", "skills/pay/SKILL.md"), reply.txt first so
//     the caller can peel the reply body off the attachments.
//   - Files returns Parts whose Filename is the path RELATIVE to task-<id> —
//     "in/invoice.pdf", "out/receipt.pdf", "transcript-<runID>.jsonl" — so the
//     archive step can tell an inbound attachment from an artifact from a
//     transcript. This is the input to archive-before-delete.
type Workspace interface {
	// Create makes task-<id>/ with empty in/ and out/. Idempotent.
	Create(taskID int64) error
	// Exists reports whether task-<id>/ has been created.
	Exists(taskID int64) bool
	// LayIn writes parts into in/, appending across turns and overwriting a
	// file of the same name (docs/05: prior + current context accumulate).
	LayIn(taskID int64, parts []mime.Part) error
	// Harvest reads out/ into parts; if a "reply.txt" is present it is returned
	// FIRST, the rest in deterministic (filename-sorted) order.
	Harvest(taskID int64) ([]mime.Part, error)
	// WriteOut writes parts into out/ — how the (sim) harness deposits a turn's
	// output. Appends across turns, overwriting same-named files.
	WriteOut(taskID int64, parts []mime.Part) error
	// ResetOut empties out/ (recreating it empty) so a turn's out/ holds ONLY what
	// that turn produced. Called at the start of every run: without it a prior
	// turn's out/reply.txt lingers and the harvest re-sends it verbatim on a
	// follow-up where the agent wrote no new reply (decision-019). in/ is
	// deliberately NOT reset — prior context accumulates there. Idempotent; a
	// task with no out/ yet is a no-op.
	ResetOut(taskID int64) error
	// WriteMemory provisions the memory collection into this run's in/ box
	// (feature-memory.md): each memory to in/memory/<name>.md (its raw Content), a
	// käsi-authored index at in/MEMORY.md (never written by the agent), and the
	// provisioned name SET for this run, recorded workspace-private so the harvest's
	// deletion diff can pin against exactly what this run was given. The manifest is
	// NOT an in/ file — Files never lists it, so archival ignores it. The notes and
	// index ARE in/ files (ordinary inputs), laid with LayIn's overwrite/append
	// semantics.
	WriteMemory(taskID int64, mems []MemoryFile) error
	// WriteApps provisions the callable-apps index into this run's in/ box as
	// in/apps.json (feature-apps.md, "the agent uses apps"): a JSON object keyed by
	// app name, each entry the app's localhost URL and its operations. Written fresh
	// every run with LayIn's overwrite semantics, so it is always current and never
	// lists a removed app; an empty set writes an empty object. It is an ordinary in/
	// input (Files lists it, archival keeps it).
	WriteApps(taskID int64, apps []AppFile) error
	// ProvisionedMemory returns the memory names WriteMemory last recorded for this
	// run — the pinned "provisioned" set the harvest diffs surviving in/memory/
	// against (forgotten = provisioned − survivors − rewritten). Absent (no memory
	// provisioned) yields an empty list, no error.
	ProvisionedMemory(taskID int64) ([]string, error)
	// DeleteIn removes a file from the in/ box by its box-relative path
	// ("memory/reply-style.md") — the way an agent forgets a memory, deleting the
	// copy it was handed (feature-memory.md). An absent file is a no-op. The path is
	// validated to stay inside the box (decision-011).
	DeleteIn(taskID int64, rel string) error
	// WriteSkills provisions skill trees into the SkillsBox (Flow D, decision-009).
	// The box is .claude/skills/ — the location the Claude CLI discovers project
	// skills from, relative to its cwd (the task dir) — so a run finds
	// ./.claude/skills/<name>/SKILL.md natively. Parts carry paths relative to the
	// box ("pay-invoice/SKILL.md", "pay-invoice/scripts/run.sh"); each lands at that
	// relative path, intermediate directories created, paths validated to stay in
	// the box (decision-011). Same overwrite/append semantics as WriteOut.
	WriteSkills(taskID int64, parts []mime.Part) error
	// WriteTranscript stores run-<runID>'s transcript bytes verbatim.
	WriteTranscript(taskID, runID int64, b []byte) error
	// ReadTranscript returns a run's transcript bytes.
	ReadTranscript(taskID, runID int64) ([]byte, error)
	// Files lists every file currently under task-<id>/ (in/, out/, and each
	// transcript) in a deterministic order, for archival. Filename is the path
	// relative to task-<id> (see the interface doc).
	Files(taskID int64) ([]mime.Part, error)
	// Delete removes task-<id>/, but ONLY after proving every current file has
	// already been archived: archived is keyed by hex sha256 of file bytes, and
	// Delete returns a non-nil error naming the first live file whose hash is
	// absent (docs/05, docs/13: never delete a workspace before its contents are
	// provably archived). Deleting an absent task is a no-op (crash-safe).
	Delete(taskID int64, archived map[string]bool) error
	// Root is the location backing this workspace (a directory for OS, a
	// synthetic sentinel for Memory).
	Root() string
}

// validBoxPath rejects a part path that would escape its box: an absolute path,
// an empty path, or one with a ".." segment. It returns the path cleaned to
// forward slashes for use as a relative box location. Both twins call it before
// writing so the sandbox boundary holds on disk and in memory alike
// (decision-011).
// SkillsBox is the workspace box skills are provisioned into (Flow D). It is
// .claude/skills/ because that is where the Claude CLI discovers project skills,
// relative to its cwd (the task dir) — so a provisioned skill is surfaced to the
// agent natively (progressive disclosure), not left in a directory it never reads.
const SkillsBox = ".claude/skills"

// MemoryDir is the in/ box subdirectory the memory notes are provisioned into,
// one file per note (feature-memory.md). MemoryIndexName is the käsi-authored
// index at the in/ box root, listing every note.
const (
	MemoryDir       = "memory"
	MemoryIndexName = "MEMORY.md"
)

// safeMemoryName reports whether a memory name yields a note path that stays inside
// the in/ box — a workspace-LOCAL guard (no memory-domain import, per the interface
// doc) that reuses the very rule validBoxPath enforces. A name with a "/", a "..",
// or anything path.Clean would rewrite is unsafe. WriteMemory drops such a name
// rather than erroring the whole provisioning: one poisoned entry (should it ever
// reach the edge past the domain's own guard) must never wedge every run
// (feature-memory.md hardening, defense in depth).
func safeMemoryName(name string) bool {
	if name == "" || strings.ContainsRune(name, '/') {
		return false
	}
	rel := MemoryDir + "/" + name + ".md"
	cleaned, err := validBoxPath("in", rel)
	return err == nil && cleaned == rel
}

// provisionableMemories keeps only the memories with a box-safe name, logging and
// dropping the rest — so a bad entry is silently skipped, never an error out of
// WriteMemory. Both memoryParts (the in/ files) and memoryNames (the pinned
// ProvisionedMemory set) run off this filtered slice, so the harvest diff stays
// consistent with what was actually written (feature-memory.md).
func provisionableMemories(mems []MemoryFile) []MemoryFile {
	out := make([]MemoryFile, 0, len(mems))
	for _, m := range mems {
		if !safeMemoryName(m.Name) {
			log.Printf("workspace: skipping memory with unsafe name %q", m.Name)
			continue
		}
		out = append(out, m)
	}
	return out
}

// memoryParts renders the in/ box files a memory provisioning lays down: one note
// per memory at memory/<name>.md (its RAW Content) plus the MEMORY.md index. Both
// twins lay these identically, so the projection is byte-for-byte the same on disk
// and in memory (the twin rule, decision-012).
func memoryParts(mems []MemoryFile) []mime.Part {
	parts := make([]mime.Part, 0, len(mems)+1)
	for _, m := range mems {
		parts = append(parts, mime.Part{
			Filename:    MemoryDir + "/" + m.Name + ".md",
			ContentType: "text/markdown; charset=utf-8",
			Bytes:       append([]byte(nil), m.Content...),
		})
	}
	parts = append(parts, mime.Part{
		Filename:    MemoryIndexName,
		ContentType: "text/markdown; charset=utf-8",
		Bytes:       memoryIndex(mems),
	})
	return parts
}

// memoryIndex renders in/MEMORY.md from the collection — the index käsi keeps in
// step with in/memory/, never written by the agent (feature-memory.md). One line
// per memory: its name linked to memory/<name>.md, then its (derived) description.
func memoryIndex(mems []MemoryFile) []byte {
	var b strings.Builder
	b.WriteString("# Memory\n\nDurable facts käsi has learned. Each links to a note in ./memory/.\n")
	if len(mems) > 0 {
		b.WriteString("\n")
	}
	for _, m := range mems {
		fmt.Fprintf(&b, "- [%s](%s/%s.md)", m.Name, MemoryDir, m.Name)
		if m.Description != "" {
			fmt.Fprintf(&b, " — %s", m.Description)
		}
		b.WriteString("\n")
	}
	return []byte(b.String())
}

// memoryNames lists the provisioned memory names in collection order — the pinned
// set ProvisionedMemory returns for the harvest's deletion diff.
func memoryNames(mems []MemoryFile) []string {
	names := make([]string, 0, len(mems))
	for _, m := range mems {
		names = append(names, m.Name)
	}
	return names
}

func validBoxPath(box, filename string) (string, error) {
	name := path.Clean(filepath.ToSlash(filename))
	if name == "" || name == "." {
		return "", fmt.Errorf("workspace: %s: empty part path %q", box, filename)
	}
	if path.IsAbs(name) || filepath.IsAbs(filename) {
		return "", fmt.Errorf("workspace: %s: absolute part path %q not allowed", box, filename)
	}
	if name == ".." || strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("workspace: %s: part path %q escapes the box", box, filename)
	}
	return name, nil
}
