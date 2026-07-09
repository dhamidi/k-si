package apps

import (
	"context"

	"github.com/dhamidi/k-si/runtime"
)

// "remove-app-unit" — stop the app and drop its systemctl --user unit, on the Runner edge; idempotent
const RemoveAppUnit = "remove-app-unit"

type RemoveAppUnitPayload struct {
	Name string `json:"name"`
}

func NewRemoveAppUnit(p RemoveAppUnitPayload) runtime.Cmd {
	return runtime.NewCmd(RemoveAppUnit, p)
}

func registerRemoveAppUnit(mod *runtime.Module) {
	runtime.HandleCmd(mod, RemoveAppUnit, removeAppUnitEffect)
}

func removeAppUnitEffect(ctx context.Context, e Edges, p RemoveAppUnitPayload,
	emit runtime.Emit) error {
	if err := e.Runner.Stop(ctx, p.Name); err != nil {
		return err // recorded; the apps-reconcile source fires again next pass (docs/15)
	}
	if err := e.Runner.Remove(ctx, p.Name); err != nil {
		return err
	}

	emit(NewMarkAppRemoved(MarkAppRemovedPayload{Name: p.Name}))
	return nil
}
