package datastore

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

// OS is the on-disk store: one persistent directory at root, symlinked into
// each run's workspace under workdir. Reads delegate to os.DirFS(root); writes
// happen through the symlink the agent follows, never through this type — the
// store is a live directory of SQLite databases and scratch files, mutated in
// place, that persists because it lives outside the ephemeral workspace, not
// because it is serialised (decision-012).
type OS struct {
	root    string
	workdir string
	fsys    fs.FS
}

var _ Store = (*OS)(nil)

// NewOS creates (once) the persistent store directory at root and prepares the
// symlinks-into-workdir binding. root is $STATE/store; workdir is $WORKDIR, the
// workspace root under which each task-<id>/ lives.
func NewOS(root, workdir string) (*OS, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("datastore: create store %q: %w", root, err)
	}
	return &OS{root: root, workdir: workdir, fsys: os.DirFS(root)}, nil
}

// Open serves a store file for reading, delegating to os.DirFS(root).
func (o *OS) Open(name string) (fs.File, error) {
	return o.fsys.Open(name)
}

// Link symlinks the store directory to task-<id>/store inside the workspace,
// idempotently. If the link already exists as a symlink it is replaced (never
// followed into or deleted at its target); if a real file or directory sits
// there instead, that is an error rather than something to clobber. The task
// dir is ensured first so the link has a home.
func (o *OS) Link(taskID int64) error {
	taskDir := filepath.Join(o.workdir, "task-"+strconv.FormatInt(taskID, 10))
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return fmt.Errorf("datastore: ensure task dir %q: %w", taskDir, err)
	}
	link := filepath.Join(taskDir, "store")

	info, err := os.Lstat(link)
	switch {
	case err == nil && info.Mode()&os.ModeSymlink != 0:
		// Already a symlink — remove only the link, never its target, then
		// recreate so a moved store root or a stale link is corrected.
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("datastore: relink %q: %w", link, err)
		}
	case err == nil:
		// A real file or directory occupies the slot — refuse to touch it.
		return fmt.Errorf("datastore: %q exists and is not a symlink; refusing to replace it", link)
	case !os.IsNotExist(err):
		return fmt.Errorf("datastore: stat %q: %w", link, err)
	}

	if err := os.Symlink(o.root, link); err != nil {
		return fmt.Errorf("datastore: symlink %q -> %q: %w", link, o.root, err)
	}
	return nil
}
