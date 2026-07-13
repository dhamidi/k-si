package skills

import "github.com/dhamidi/k-si/runtime"

// Edges is everything skills touches in the world. Real implementations are
// wired in cmd/kasi/main.go; simulated twins live in this package (docs/12).
type Edges struct {
	Clock runtime.Clock
}

// Module bundles Agent-authored skills registry (Flow D, docs/07) (docs/01).
func Module(e Edges) *runtime.Module {
	mod := runtime.NewModule("skills", Model{}, e)

	registerRegisterSkill(mod)
	registerUnregisterSkill(mod)
	return mod
}

// SimEdges is the full simulated set — what `kasi test` assembles by
// default, and the simulated twin the twin rule demands (docs/12).
func SimEdges() Edges {
	return Edges{
		Clock: runtime.SimClock(),
	}
}
