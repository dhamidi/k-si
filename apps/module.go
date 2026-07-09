package apps

import (
	"github.com/dhamidi/k-si/apprunner"
	"github.com/dhamidi/k-si/runtime"
)

// Edges is everything apps touches in the world. Real implementations are
// wired in cmd/kasi/main.go; simulated twins live in this package (docs/12).
type Edges struct {
	// keeps an app's process up under systemd --user: write/start/stop/remove its unit, read status + journald logs
	Runner apprunner.Runner
	Clock  runtime.Clock
}

// Module bundles the app registry: name -> port, start command, operations; rebuilt by replay (feature-apps.md) (docs/01).
func Module(e Edges) *runtime.Module {
	mod := runtime.NewModule("apps", Model{}, e)

	registerRegisterApp(mod)
	registerUnregisterApp(mod)
	runtime.Subscribe(mod, appsReconcileSubs)
	registerRunApp(mod)
	registerRetireApp(mod)
	registerInstallAppUnit(mod)
	registerMarkAppRunning(mod)
	registerRemoveAppUnit(mod)
	registerMarkAppRemoved(mod)
	return mod
}

// SimEdges is the full simulated set — what `kasi test` assembles by
// default, and the simulated twin the twin rule demands (docs/12).
func SimEdges() Edges {
	return Edges{
		Runner: apprunner.NewSim(),
		Clock:  runtime.SimClock(),
	}
}
