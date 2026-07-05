package runtime

import (
	"sync"
	"time"
)

// Clock is the time edge. Handlers never read it — time enters the model on
// messages, stamped by effects and subscriptions at the edge (docs/01).
type Clock interface {
	Now() time.Time
}

// RealClock is the wall clock, wired in main.go for the live assembly.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

// SimulatedClock is the virtual clock: it moves only when a scenario says
// `advance` (docs/13). Its zero point is fixed so runs are reproducible.
type SimulatedClock struct {
	mu  sync.Mutex
	now time.Time
}

// SimClock returns a simulated clock at the fixed epoch.
func SimClock() *SimulatedClock {
	return &SimulatedClock{now: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *SimulatedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves virtual time forward; nothing else ever does.
func (c *SimulatedClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
