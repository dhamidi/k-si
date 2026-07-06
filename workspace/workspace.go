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
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/dhamidi/k-si/mime"
)

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
