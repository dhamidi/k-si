package apprunner

import (
	"context"
	"fmt"
	"sync"
)

// Sim is the in-memory twin of the systemd Runner (docs/12): it records the
// unit state a scenario would produce on a real host, so the apps-reconcile
// loop converges (Install→Start makes Status report up, which lets
// mark-app-running fire) without touching the machine. Concurrency-safe: sim
// edges drive effects from scenario goroutines.
type Sim struct {
	mu    sync.Mutex
	units map[string]*simUnit
}

type simUnit struct {
	port     int
	startCmd string
	up       bool
	logs     []string
}

// NewSim builds an empty Runner twin.
func NewSim() *Sim {
	return &Sim{units: make(map[string]*simUnit)}
}

func (s *Sim) ensure(name string) *simUnit {
	u := s.units[name]
	if u == nil {
		u = &simUnit{}
		s.units[name] = u
	}
	return u
}

// Install records (or replaces) the unit; idempotent, like the real adapter.
func (s *Sim) Install(ctx context.Context, name string, port int, startCmd string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.ensure(name)
	u.port = port
	u.startCmd = startCmd
	u.logs = append(u.logs, fmt.Sprintf("installed %s on port %d: %s", name, port, startCmd))
	return nil
}

// Start marks the unit up.
func (s *Sim) Start(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.ensure(name)
	u.up = true
	u.logs = append(u.logs, "started")
	return nil
}

// Stop marks the unit down without forgetting it.
func (s *Sim) Stop(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u := s.units[name]; u != nil {
		u.up = false
		u.logs = append(u.logs, "stopped")
	}
	return nil
}

// Remove forgets the unit; a no-op if already gone.
func (s *Sim) Remove(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.units, name)
	return nil
}

// Status is up only while a unit exists and is started.
func (s *Sim) Status(ctx context.Context, name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.units[name]
	return u != nil && u.up, nil
}

// Logs returns at most the last n recorded lines, oldest first.
func (s *Sim) Logs(ctx context.Context, name string, n int) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.units[name]
	if u == nil {
		return nil, nil
	}
	if n <= 0 || n >= len(u.logs) {
		return append([]string(nil), u.logs...), nil
	}
	return append([]string(nil), u.logs[len(u.logs)-n:]...), nil
}
