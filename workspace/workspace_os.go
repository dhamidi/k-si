package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"github.com/dhamidi/k-si/mime"
)

// OS is the on-disk twin of Memory: a $WORKDIR the harness and the main agent
// share on a real filesystem (docs/05). Stage 1 wires only the sim ring, so the
// I/O methods are stage-2 stubs — enough shape to compile and be held in a
// module's Edges, filled in when the serve path lands. Root and Exists are
// already real so the runner can probe the directory.
type OS struct {
	root string
}

var _ Workspace = (*OS)(nil)

// errStage2 marks the not-yet-implemented on-disk operations.
var errStage2 = errors.New("workspace: OS twin not implemented (stage 2)")

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

func (o *OS) Create(taskID int64) error                           { return errStage2 }
func (o *OS) LayIn(taskID int64, parts []mime.Part) error         { return errStage2 }
func (o *OS) Harvest(taskID int64) ([]mime.Part, error)           { return nil, errStage2 }
func (o *OS) WriteOut(taskID int64, parts []mime.Part) error      { return errStage2 }
func (o *OS) WriteTranscript(taskID, runID int64, b []byte) error { return errStage2 }
func (o *OS) ReadTranscript(taskID, runID int64) ([]byte, error)  { return nil, errStage2 }
func (o *OS) Files(taskID int64) ([]mime.Part, error)             { return nil, errStage2 }
func (o *OS) Delete(taskID int64, archived map[string]bool) error { return errStage2 }
