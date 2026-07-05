// Package workspace is the per-task scratch-directory edge (docs/05): the
// filesystem seam where the main agent lays inbound mail into a task's in/,
// runs a harness, and harvests out/ into a reply. Domains hold a Workspace in
// their Edges. Two twins implement it — Memory (the simulation ring, an
// in-memory tree that SURVIVES a simulated crash the way a real directory
// would) and OS ($WORKDIR on disk, stage-2 wiring). Only the mime object model
// (docs/02) and the standard library are imported; nothing here touches the
// runtime.
package workspace

import "github.com/dhamidi/k-si/mime"

// Workspace is the filesystem edge for a task's ephemeral scratch directory,
// laid out as $WORKDIR/task-<id>/{in,out}/ plus per-run transcripts (docs/05).
//
// Filename conventions on the mime.Part values that cross this seam:
//
//   - LayIn and WriteOut take Parts with PLAIN filenames ("body.txt",
//     "invoice.pdf", "reply.txt"); they land under in/ and out/ respectively.
//   - Harvest returns Parts with PLAIN filenames (out/'s contents), reply.txt
//     first so the caller can peel the reply body off the attachments.
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
