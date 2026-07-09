package apps

import (
	"github.com/dhamidi/k-si/apps/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "register-app" — record a registered app (add, or replace-in-place on re-add); keyed by name like remember updates a memory

func registerRegisterApp(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.RegisterApp, handleRegisterApp)
}

func handleRegisterApp(v runtime.View, s Model, p msg.RegisterAppPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	entry := App{
		Name:       p.Name,
		Port:       p.Port,
		StartCmd:   p.StartCmd,
		Operations: p.Operations,
		URL:        p.URL,
		Status:     StatusRegistered,
	}

	// A name is UNIQUE (feature-apps.md): re-registering an existing name
	// replaces that app's entry in place rather than appending a duplicate.
	// Copy-on-write so the prior model stays untouched for lock-free readers
	// and replay (rules/no-inplace-model-mutation.yml).
	next := append([]App(nil), s.Apps...)
	if i := s.findName(p.Name); i >= 0 {
		next[i] = entry
	} else {
		next = append(next, entry)
	}
	s.Apps = next

	// No command: registering only records intent. The apps-reconcile
	// subscription is the sole launcher (decision-015), so a crash between
	// this append and the unit write is recovered on replay.
	return s, nil
}
