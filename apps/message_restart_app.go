package apps

import (
	"github.com/dhamidi/k-si/apps/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "restart-app" — bounce a running app's unit so it picks up new code; injected by the /control/app endpoint

func registerRestartApp(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.RestartApp, handleRestartApp)
}

// handleRestartApp changes NO model state — a restart leaves the registry entry
// exactly as it was (still `running`). It exists purely to drive the bounce
// through the log for auditability, returning the restart-app-unit command that
// does the machine-side work on the Runner edge. A missing app is a no-op.
func handleRestartApp(v runtime.View, s Model, p msg.RestartAppPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	i := s.findName(p.Name)
	if i < 0 {
		return s, nil
	}

	return s, []runtime.Cmd{
		NewRestartAppUnit(RestartAppUnitPayload{Name: s.Apps[i].Name}),
	}
}
