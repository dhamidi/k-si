package apps

import (
	"context"

	"github.com/dhamidi/k-si/runtime"
)

// "install-app-unit" — write the systemctl --user unit for the app and start it, on the Runner edge; idempotent
const InstallAppUnit = "install-app-unit"

type InstallAppUnitPayload struct {
	Name     string `json:"name"`
	Port     int    `json:"port"`
	StartCmd string `json:"start_cmd"`
}

func NewInstallAppUnit(p InstallAppUnitPayload) runtime.Cmd {
	return runtime.NewCmd(InstallAppUnit, p)
}

func registerInstallAppUnit(mod *runtime.Module) {
	runtime.HandleCmd(mod, InstallAppUnit, installAppUnitEffect)
}

func installAppUnitEffect(ctx context.Context, e Edges, p InstallAppUnitPayload,
	emit runtime.Emit) error {
	if err := e.Runner.Install(ctx, p.Name, p.Port, p.StartCmd); err != nil {
		return err // recorded; the apps-reconcile source fires again next pass (docs/15)
	}
	if err := e.Runner.Start(ctx, p.Name); err != nil {
		return err
	}

	emit(NewMarkAppRunning(MarkAppRunningPayload{Name: p.Name}))
	return nil
}
