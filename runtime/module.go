package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

// Module bundles everything a domain contributes (docs/01): message
// handlers, command effects, subscription providers, the domain's model
// slice, and its edges. main.go is the only place modules are assembled
// into an App — no init() registration, no globals (docs/09, docs/15).
type Module struct {
	name       string
	zero       any
	edges      any
	handlers   map[string]rawHandler
	effects    map[string]rawEffect
	prototypes map[string]func() any
	subs       []subProvider
}

type rawHandler func(v View, slice any, payload json.RawMessage, meta Meta) (any, []Cmd, bool)

type rawEffect func(ctx context.Context, edges any, payload json.RawMessage, emit Emit) error

type subProvider func(v View, slice any) []Sub

// NewModule starts a module with its name, the zero value of its model
// slice, and its edges (real or simulated — chosen by the assembly).
func NewModule(name string, zero any, edges any) *Module {
	return &Module{
		name:       name,
		zero:       zero,
		edges:      edges,
		handlers:   map[string]rawHandler{},
		effects:    map[string]rawEffect{},
		prototypes: map[string]func() any{},
	}
}

// Name is the module's stable name — its directory, its slice key.
func (m *Module) Name() string { return m.name }

// HandleMsg registers the pure handler for a message tag. The wrapper
// decodes the payload — a decode failure drops the message, recorded, never
// a panic — hands the handler a read-only View plus its own slice, and puts
// the returned slice back. Write-ownership is the signature (docs/15).
func HandleMsg[S, P any](mod *Module, tag string, fn func(v View, s S, p P, meta Meta) (S, []Cmd)) {
	mod.prototypes[tag] = func() any { return new(P) }
	mod.handlers[tag] = func(v View, slice any, payload json.RawMessage, meta Meta) (any, []Cmd, bool) {
		var p P
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &p); err != nil {
				return slice, nil, false
			}
		}

		s, _ := slice.(S)
		next, cmds := fn(v, s, p, meta)
		return next, cmds, true
	}
}

// HandleCmd registers the effect for a command tag. Effects see edges and
// payload — never the model; results leave only as emitted messages
// (docs/15).
func HandleCmd[E, P any](mod *Module, tag string, fn func(ctx context.Context, e E, p P, emit Emit) error) {
	mod.effects[tag] = func(ctx context.Context, edges any, payload json.RawMessage, emit Emit) error {
		var p P
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &p); err != nil {
				return fmt.Errorf("decode %q payload: %w", tag, err)
			}
		}

		e, _ := edges.(E)
		return fn(ctx, e, p, emit)
	}
}

// Subscribe registers a subscription provider: a pure function from state to
// the set of sources that should be running (docs/01).
func Subscribe[S any](mod *Module, fn func(v View, s S) []Sub) {
	mod.subs = append(mod.subs, func(v View, slice any) []Sub {
		s, _ := slice.(S)
		return fn(v, s)
	})
}

// strictDecode checks a payload against the tag's registered payload struct,
// rejecting unknown fields. Production tolerates drift for old logs' sake;
// the test runner never does (docs/14).
func (m *Module) strictDecode(tag string, payload json.RawMessage) error {
	proto, ok := m.prototypes[tag]
	if !ok {
		return fmt.Errorf("no payload registered for %q", tag)
	}

	if len(payload) == 0 {
		return nil
	}

	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()

	if err := dec.Decode(proto()); err != nil {
		return fmt.Errorf("payload for %q: %w", tag, err)
	}

	return nil
}
