package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// App is one assembled instance of the application: the reducer loop, the
// command interpreter, and the subscription lifecycle over a set of modules
// (docs/01). Instances are values — any number can coexist in one process,
// which is what the test runner's fleets rely on (docs/13).
type App struct {
	modules  []*Module
	msgOwner map[string]*Module
	cmdOwner map[string]*Module

	log   Log
	clock Clock

	// model is the whole application state as ONE immutable snapshot, swapped
	// atomically by the single reducer goroutine. Readers Load it lock-free and
	// get a stable, consistent value (docs/01) — the Go translation of Elm's
	// runtime holding the model value and swapping it on update. Handlers must
	// still return a NEW slice (copy-on-write): the snapshot's immutability is
	// only real if nothing mutates a slice already inside a Stored snapshot.
	model atomic.Pointer[snapshot]

	// mu guards the reducer's bookkeeping — NOT the model (that is the atomic
	// above): the pending counter and its cond, the command trace, the
	// dead-send/failure logs, and the running-subscription set.
	mu      sync.Mutex
	cond    *sync.Cond
	pending int // queued + applying + in-flight effects; 0 == quiescent
	trace   []string
	// deadSends are messages that reached no handler (or would not decode) —
	// a mistyped or mismatched tag. Fatal in a full assembly (docs/13). failures
	// are unknown commands and effects that returned an error — recorded, but
	// left to reconciliation, so fault-injection scenarios (a failed send that
	// is later retried) are not mistaken for architectural bugs.
	deadSends []string
	failures  []string
	running   map[string]runningSub
	live      bool

	inbox  chan envelope
	cancel context.CancelFunc
	done   chan struct{}
}

type envelope struct {
	msg     Msg
	cause   int64
	applied chan struct{}
}

// snapshot is an immutable point-in-time model: each module's slice keyed by
// name. Once Stored it is never mutated; an update copies the map and replaces
// one entry, so a reader holding an older snapshot is unaffected.
type snapshot struct {
	slices map[string]any
}

// with returns a new snapshot with one module's slice replaced.
func (s *snapshot) with(name string, slice any) *snapshot {
	next := make(map[string]any, len(s.slices))
	for k, v := range s.slices {
		next[k] = v
	}
	next[name] = slice
	return &snapshot{slices: next}
}

// New assembles an App from modules. This is called from main.go — the one
// assembly point (docs/01) — and from the test runner with simulated edges.
// A tag owned by two modules is an assembly error and fails loudly.
func New(modules ...*Module) *App {
	a := &App{
		modules:  modules,
		msgOwner: map[string]*Module{},
		cmdOwner: map[string]*Module{},
		clock:    RealClock{},
		running:  map[string]runningSub{},
		inbox:    make(chan envelope, 1024),
		done:     make(chan struct{}),
	}
	a.cond = sync.NewCond(&a.mu)

	slices := make(map[string]any, len(modules))
	for _, m := range modules {
		slices[m.name] = m.zero

		for tag := range m.handlers {
			if owner, taken := a.msgOwner[tag]; taken {
				panic(fmt.Sprintf("runtime: tag %q owned by both %s and %s — one tag, one owner (docs/01)", tag, owner.name, m.name))
			}
			a.msgOwner[tag] = m
		}

		for tag := range m.effects {
			if owner, taken := a.cmdOwner[tag]; taken {
				panic(fmt.Sprintf("runtime: command %q owned by both %s and %s (docs/01)", tag, owner.name, m.name))
			}
			a.cmdOwner[tag] = m
		}
	}

	a.model.Store(&snapshot{slices: slices})
	return a
}

// UseLog sets the message log; the default is nothing, and Start requires
// one. Production wires SQLite; the sim ring wires the in-memory twin.
func (a *App) UseLog(log Log) *App {
	a.log = log
	return a
}

// UseClock sets the time edge; the sim ring wires a SimulatedClock.
func (a *App) UseClock(c Clock) *App {
	a.clock = c
	return a
}

// Start rebuilds the model by folding the entire log with effects suppressed
// (docs/01: there are no snapshots), then switches live and starts the
// reducer loop and subscriptions.
func (a *App) Start(ctx context.Context) error {
	if a.log == nil {
		return fmt.Errorf("runtime: Start needs a Log (UseLog)")
	}

	if err := a.replay(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.live = true

	go a.loop(ctx)
	a.mu.Lock()
	a.diffSubscriptions(ctx)
	a.mu.Unlock()

	return nil
}

// Replay folds the log into the model without going live — no effects, no
// subscriptions, no loop. The test runner uses it for the convergence check
// that runs after every scenario (docs/13).
func (a *App) Replay() error {
	if a.log == nil {
		return fmt.Errorf("runtime: Replay needs a Log (UseLog)")
	}
	return a.replay()
}

func (a *App) replay() error {
	return a.log.Replay(func(msg Msg, meta Meta) error {
		a.apply(msg, meta)
		return nil
	})
}

// Stop cancels the loop, effects, and subscriptions. The log and whatever it
// references survive; the model does not — that is the point (docs/01).
func (a *App) Stop() {
	if a.cancel != nil {
		a.cancel()
		<-a.done
	}
}

// Send injects one message through the front door and blocks until the
// reducer has applied it — which is what lets the web edge redirect to a
// GET that renders the new model (docs/08, docs/15).
func (a *App) Send(m Msg) {
	applied := make(chan struct{})
	a.enqueue(envelope{msg: m, applied: applied})
	<-applied
}

// Settle blocks until the instance is quiescent: inbound channel empty, no
// message mid-apply, no effect in flight (docs/13). Stimuli in the test
// language settle before returning, so scripts never race the runtime.
func (a *App) Settle() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for a.pending > 0 {
		a.cond.Wait()
	}
}

func (a *App) enqueue(e envelope) {
	a.mu.Lock()
	a.pending++
	a.mu.Unlock()

	a.inbox <- e
}

func (a *App) settleOne() {
	a.mu.Lock()
	a.pending--
	if a.pending == 0 {
		a.cond.Broadcast()
	}
	a.mu.Unlock()
}

func (a *App) loop(ctx context.Context) {
	defer close(a.done)

	for {
		select {
		case <-ctx.Done():
			return
		case e := <-a.inbox:
			meta, err := a.log.Append(e.msg, e.cause, a.clock.Now())
			if err != nil {
				a.recordFailure(fmt.Sprintf("%s (log append failed: %v)", e.msg.Tag, err))
			} else {
				cmds := a.apply(e.msg, meta)
				a.interpret(ctx, cmds, meta)
			}

			if e.applied != nil {
				close(e.applied)
			}
			a.settleOne()
		}
	}
}

// apply is the fold: one message through its owning handler. Used verbatim
// by live operation and by replay; only interpretation differs (docs/01).
func (a *App) apply(msg Msg, meta Meta) []Cmd {
	owner, ok := a.msgOwner[msg.Tag]
	if !ok {
		a.recordDeadSend(msg.Tag)
		return nil
	}

	// Read the current snapshot lock-free; the reducer is the only writer, so a
	// plain Load/Store (no CAS) is correct.
	cur := a.model.Load()
	next, cmds, decoded := owner.handlers[msg.Tag](View{slices: cur.slices}, cur.slices[owner.name], msg.Payload, meta)
	if !decoded {
		a.recordDeadSend(fmt.Sprintf("%s (payload did not decode)", msg.Tag))
		return nil
	}
	a.model.Store(cur.with(owner.name, next))

	if a.live {
		a.mu.Lock()
		for _, c := range cmds {
			a.trace = append(a.trace, traceEntry(c))
		}
		a.mu.Unlock()
	}

	return cmds
}

// interpret performs commands in live mode: send injects its message onto
// the channel; every other command runs its owning effect in a worker
// goroutine, feeding results back only via emit (docs/01).
func (a *App) interpret(ctx context.Context, cmds []Cmd, meta Meta) {
	for _, c := range cmds {
		if inner, ok := decodeSend(c); ok {
			a.enqueue(envelope{msg: inner, cause: meta.Offset})
			continue
		}

		owner, ok := a.cmdOwner[c.Tag]
		if !ok {
			a.recordFailure(fmt.Sprintf("%s (command)", c.Tag))
			continue
		}

		a.mu.Lock()
		a.pending++
		a.mu.Unlock()

		go func(c Cmd, owner *Module) {
			defer a.settleOne()

			emit := func(m Msg) { a.enqueue(envelope{msg: m, cause: meta.Offset}) }
			if err := owner.effects[c.Tag](ctx, owner.edges, c.Payload, emit); err != nil {
				a.recordFailure(fmt.Sprintf("%s (effect failed: %v)", c.Tag, err))
			}
		}(c, owner)
	}

	a.mu.Lock()
	a.diffSubscriptions(ctx)
	a.mu.Unlock()
}

// diffSubscriptions reconciles declared sources against running ones by ID
// (docs/01). Callers hold a.mu.
func (a *App) diffSubscriptions(ctx context.Context) {
	type declared struct {
		sub   Sub
		edges any
	}

	desired := map[string]declared{}
	cur := a.model.Load()
	view := View{slices: cur.slices}

	for _, m := range a.modules {
		for _, provide := range m.subs {
			for _, sub := range provide(view, cur.slices[m.name]) {
				desired[sub.ID] = declared{sub: sub, edges: m.edges}
			}
		}
	}

	for id, running := range a.running {
		if _, still := desired[id]; !still {
			running.cancel()
			// A cancelled source may still emit a final message as its Run unwinds:
			// agent-watch emits finish-agent-run when a stop (or crash) cancels its
			// Wait, and in the live ring that emit trails the real process's death by
			// however long it takes to die. Hold quiescence until the Run returns so
			// Settle covers that trailing emit instead of racing it (docs/13). A
			// source that already finished has its done closed, so this is a no-op.
			a.pending++
			go func(done chan struct{}) {
				<-done
				a.settleOne()
			}(running.done)
			delete(a.running, id)
		}
	}

	for id, d := range desired {
		if _, already := a.running[id]; already {
			continue
		}

		subCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		a.running[id] = runningSub{cancel: cancel, done: done}

		// An Await source is pending work until its Run returns (docs/13), so a
		// reconciliation that emits then finishes is drained before Settle. We
		// hold a.mu here, so this bump is safe; settleOne takes the lock itself.
		if d.sub.Await {
			a.pending++
		}

		go func(d declared) {
			defer close(done)
			if d.sub.Await {
				defer a.settleOne()
			}
			d.sub.Run(subCtx, d.edges, func(m Msg) { a.enqueue(envelope{msg: m, cause: 0}) })
		}(d)
	}
}

func (a *App) recordDeadSend(entry string) {
	a.mu.Lock()
	a.deadSends = append(a.deadSends, entry)
	a.mu.Unlock()
}

func (a *App) recordFailure(entry string) {
	a.mu.Lock()
	a.failures = append(a.failures, entry)
	a.mu.Unlock()
}

func traceEntry(c Cmd) string {
	if inner, ok := decodeSend(c); ok {
		return "send:" + inner.Tag
	}
	return c.Tag
}

// --- Introspection for edges and the test runner -----------------------------

// View returns a read snapshot of the model for edge reads (docs/08) — a
// lock-free atomic Load of the current immutable snapshot, which stays stable
// while the reducer swaps in later ones.
func (a *App) View() View {
	return View{slices: a.model.Load().slices}
}

// HasTag reports whether any assembled module handles the message tag.
func (a *App) HasTag(tag string) bool {
	_, ok := a.msgOwner[tag]
	return ok
}

// StrictDecode validates a payload against the tag's registered payload
// struct, rejecting unknown fields (docs/14).
func (a *App) StrictDecode(tag string, payload json.RawMessage) error {
	owner, ok := a.msgOwner[tag]
	if !ok {
		return fmt.Errorf("no module in this assembly handles %q", tag)
	}
	return owner.strictDecode(tag, payload)
}

// ModelJSON returns a module's slice as JSON, for generic reads and the
// replay-convergence check.
func (a *App) ModelJSON(module string) ([]byte, error) {
	slice, ok := a.model.Load().slices[module]
	if !ok {
		return nil, fmt.Errorf("no module %q in this assembly", module)
	}
	return json.Marshal(slice)
}

// ModuleNames lists the assembled modules in assembly order.
func (a *App) ModuleNames() []string {
	names := make([]string, 0, len(a.modules))
	for _, m := range a.modules {
		names = append(names, m.name)
	}
	return names
}

// DrainTrace returns and clears the recorded command trace (docs/14:
// `commands` reads what handlers returned since last read).
func (a *App) DrainTrace() []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	t := a.trace
	a.trace = nil
	return t
}

// Dropped lists messages that reached no handler (unhandled or undecodable
// tags) — the dead sends. Expected at a partial assembly's boundary; fatal in
// a full one (docs/13). Effect failures are NOT here — see Failures.
func (a *App) Dropped() []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]string(nil), a.deadSends...)
}

// Failures lists unknown commands and effects that returned an error. Unlike a
// dead send, a failure is not a wiring bug: it is what reconciliation exists to
// recover from (a send that failed and will be retried — docs/03), so it is
// recorded for diagnostics but never fails a scenario on its own.
func (a *App) Failures() []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]string(nil), a.failures...)
}
