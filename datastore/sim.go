package datastore

import (
	"io"
	"io/fs"
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

// Link is a no-op: an in-memory store has no filesystem to symlink into.
func (s *Sim) Link(taskID int64) error { return nil }

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

// simFileInfo reports the metadata fs.ReadFile and friends inspect.
type simFileInfo struct {
	name string
	size int64
}

func (i simFileInfo) Name() string       { return i.name }
func (i simFileInfo) Size() int64        { return i.size }
func (i simFileInfo) Mode() fs.FileMode  { return 0o444 }
func (i simFileInfo) ModTime() time.Time { return time.Time{} }
func (i simFileInfo) IsDir() bool        { return false }
func (i simFileInfo) Sys() any           { return nil }
