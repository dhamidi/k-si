// Package datastore is the agent's persistent store — one directory that lives
// OUTSIDE the event log and OUTSIDE the ephemeral per-task workspace, symlinked
// into every run at ./store/ (Flow F, decision-012). käsi provisions and keeps
// the directory but never event-sources its contents: it is external mutable
// state reached through an edge, exactly like the mail edge. Reads go through
// fs.FS; the agent writes directly via the symlink; archival skips the link.
package datastore

import "io/fs"

// Store is the store edge. It is read through fs.FS (os.DirFS in production) and
// exposed into a run's workspace as a symlink by Link. The agent mutates the
// live directory directly through that link — writes are not part of this
// interface, which is käsi's read-only-plus-provision view of the store.
type Store interface {
	fs.FS
	// Link exposes the store at task-<id>/store inside the workspace so the run
	// can read and write it directly. Idempotent: relinking a run is safe.
	Link(taskID int64) error
}
