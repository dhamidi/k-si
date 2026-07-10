package datastore

import (
	"io"
	"io/fs"
	"sort"
	"strings"
	"sync"
	"time"
)

// Sim is the in-memory store twin (the twin rule, docs/12): one map shared
// across every task in a scenario, exactly as the OS store's single directory
// is shared across tasks. Link is a no-op — there is no filesystem to symlink
// into — so a scenario proving persistence asserts on the store's observable
// contents, not on a rebuilt model (decision-012).
type Sim struct {
	mu    sync.RWMutex
	files map[string][]byte
}

var _ Store = (*Sim)(nil)

// NewSim builds an empty shared in-memory store.
func NewSim() *Sim {
	return &Sim{files: map[string][]byte{}}
}

// Write seeds a file into the store, simulating the agent having cached data
// through the ./store symlink. Used by scenario seeding and future harness
// twins; it is not part of the Store interface käsi wires as an edge.
func (s *Sim) Write(name string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := make([]byte, len(data))
	copy(b, data)
	s.files[name] = b
	return nil
}

// Open serves a stored file for reading. A miss is fs.ErrNotExist wrapped in a
// *fs.PathError, matching os.DirFS.
func (s *Sim) Open(name string) (fs.File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.files[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	data := make([]byte, len(b))
	copy(data, b)
	return &simFile{name: name, data: data}, nil
}

// ReadDir lists a directory's immediate children, synthesizing directory
// entries from the flat key space so the in-memory twin walks like os.DirFS —
// the production store — does: a caller reading the store sees folders and
// files, not one flat list (the twin rule, docs/12). name is a slash path, or
// "." for the root. It satisfies fs.ReadDirFS, which is what fs.ReadDir calls.
// A name that is a file (or absent under the root) is a not-a-directory error,
// matching os.DirFS.
func (s *Sim) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := ""
	if name != "." {
		prefix = name + "/"
	}
	files := map[string]int64{}
	dirs := map[string]bool{}
	found := false
	for key, data := range s.files {
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			continue
		}
		found = true
		rest := key[len(prefix):]
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			dirs[rest[:i]] = true
		} else {
			files[rest] = int64(len(data))
		}
	}
	// A named subpath with no keys beneath it is not a directory. The root is
	// always a directory, empty or not.
	if name != "." && !found {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}

	entries := make([]fs.DirEntry, 0, len(files)+len(dirs))
	for d := range dirs {
		entries = append(entries, simDirEntry{name: d, dir: true})
	}
	for f, size := range files {
		entries = append(entries, simDirEntry{name: f, size: size})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

// Link is a no-op: an in-memory store has no filesystem to symlink into.
func (s *Sim) Link(taskID int64) error { return nil }

// simDirEntry is one child ReadDir reports — a stored file or a synthesized
// directory inferred from a deeper key.
type simDirEntry struct {
	name string
	dir  bool
	size int64
}

func (e simDirEntry) Name() string { return e.name }
func (e simDirEntry) IsDir() bool  { return e.dir }
func (e simDirEntry) Type() fs.FileMode {
	if e.dir {
		return fs.ModeDir
	}
	return 0
}
func (e simDirEntry) Info() (fs.FileInfo, error) {
	return simFileInfo{name: e.name, size: e.size, dir: e.dir}, nil
}

// simFile is a read-only open handle over a copy of a stored file's bytes.
type simFile struct {
	name string
	data []byte
	off  int
}

func (f *simFile) Stat() (fs.FileInfo, error) {
	return simFileInfo{name: f.name, size: int64(len(f.data))}, nil
}

func (f *simFile) Read(p []byte) (int, error) {
	if f.off >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.off:])
	f.off += n
	return n, nil
}

func (f *simFile) Close() error { return nil }

// simFileInfo reports the metadata fs.ReadFile and friends inspect. dir marks a
// synthesized directory (ReadDir), which the flat store has no stored bytes for.
type simFileInfo struct {
	name string
	size int64
	dir  bool
}

func (i simFileInfo) Name() string { return i.name }
func (i simFileInfo) Size() int64  { return i.size }
func (i simFileInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
func (i simFileInfo) ModTime() time.Time { return time.Time{} }
func (i simFileInfo) IsDir() bool        { return i.dir }
func (i simFileInfo) Sys() any           { return nil }
