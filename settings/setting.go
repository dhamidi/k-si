// Package settings is käsi's runtime form engine and the typed-setting
// contribution model (docs/16, decision-020). A module contributes a Setting —
// a read plus a write over configuration whose STATE stays in that module's own
// slice — and the value's Go type drives BOTH its form and its parse (the "shape
// defined once" goal). It is a leaf package: it imports only runtime/, so any
// domain can build a descriptor without an import cycle (docs/09), exactly like
// the msg/ leaves and testlang/.
package settings

import "github.com/dhamidi/k-si/runtime"

// Setting is one typed, editable piece of configuration a module contributes.
// The value's STATE stays in the owning module's slice — contribution is not
// relocation (docs/16). The descriptor is a read plus a write over that state,
// assembled in main.go beside the module list it mirrors (docs/01); there is no
// global registry and no init().
type Setting struct {
	Key   string // stable id, snake_case; the route param and the form's scope
	Short string // one line, shown in the settings list
	Long  string // help text, shown on the form
	Owner string // the owning module's name (email, tasks, agents, admin)

	// Read pulls the current typed value out of the model, through the owning
	// domain's pure View read helper — a read like any other (docs/15).
	Read func(v runtime.View) Value

	// Write turns an accepted value into a set-* message — the one imperative
	// write, logged and replayable (decision-001). An existing message where the
	// setting maps to one; a new whole-value message where it does not (a
	// list-replacing set-allowlist, not an incremental allow-sender).
	Write func(Value) runtime.Msg
}

// Value is a setting's typed value — a domain type (an Allowlist, a BaseURL, a
// MaxConcurrent), never a stringly map. It knows how to build its own form; that
// form knows how to parse back into a Value. One shape, two directions.
type Value interface {
	ToFormer
}
