package apps

import "github.com/dhamidi/k-si/runtime"

// "run-app" — reconcile-driven: turn a registered app into its install-app-unit effect
const RunApp = "run-app"

type RunAppPayload struct {
	Name string `json:"name"`
}

func NewRunApp(p RunAppPayload) runtime.Msg {
	return runtime.NewMsg(RunApp, p)
}

func registerRunApp(mod *runtime.Module) {
	runtime.HandleMsg(mod, RunApp, handleRunApp)
}

func handleRunApp(v runtime.View, s Model, p RunAppPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	i := s.findName(p.Name)
	if i < 0 || s.Apps[i].Status != StatusRegistered {
		return s, nil
	}

	app := s.Apps[i]
	return s, []runtime.Cmd{
		NewInstallAppUnit(InstallAppUnitPayload{
			Name:     app.Name,
			Port:     app.Port,
			StartCmd: app.StartCmd,
		}),
	}
}
