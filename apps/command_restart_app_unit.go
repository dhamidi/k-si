package apps

import (
	"context"

	"github.com/dhamidi/k-si/runtime"
)

// "restart-app-unit" — bounce the app's systemctl --user unit on the Runner edge; idempotent, a missing unit is a no-op
const RestartAppUnit = "restart-app-unit"

type RestartAppUnitPayload struct {
	Name string `json:"name"`
}

func NewRestartAppUnit(p RestartAppUnitPayload) runtime.Cmd {
	return runtime.NewCmd(RestartAppUnit, p)
}

func registerRestartAppUnit(mod *runtime.Module) {
	runtime.HandleCmd(mod, RestartAppUnit, restartAppUnitEffect)
}

func restartAppUnitEffect(ctx context.Context, e Edges, p RestartAppUnitPayload,
	emit runtime.Emit) error {
	// The bounce is the whole effect: it emits nothing, because a restart changes
	// no registry state (the app stays `running`). A failure is recorded and the
	// directive stands in the log for a re-drive.
	return e.Runner.Restart(ctx, p.Name)
}
