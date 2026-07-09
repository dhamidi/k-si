package apps

import (
	"github.com/dhamidi/k-si/apps/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "unregister-app" — mark an app for removal (rm); the unit is torn down by reconciliation, then the entry is dropped

func registerUnregisterApp(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.UnregisterApp, handleUnregisterApp)
}

func handleUnregisterApp(v runtime.View, s Model, p msg.UnregisterAppPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	i := s.findName(p.Name)
	if i < 0 {
		return s, nil
	}

	// Copy-on-write (rules/no-inplace-model-mutation.yml): mark removing
	// without touching the prior model, same as register-app.
	next := append([]App(nil), s.Apps...)
	next[i].Status = StatusRemoving
	s.Apps = next

	// No command: removal only records intent. The apps-reconcile
	// subscription is the sole teardown path (decision-015-style), so a
	// crash mid-teardown re-asserts on replay. mark-app-removed drops the
	// entry once the unit is actually gone.
	return s, nil
}
