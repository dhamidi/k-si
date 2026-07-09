package apps

import "github.com/dhamidi/k-si/runtime"

// "retire-app" — reconcile-driven: turn a removing app into its remove-app-unit effect
const RetireApp = "retire-app"

type RetireAppPayload struct {
	Name string `json:"name"`
}

func NewRetireApp(p RetireAppPayload) runtime.Msg {
	return runtime.NewMsg(RetireApp, p)
}

func registerRetireApp(mod *runtime.Module) {
	runtime.HandleMsg(mod, RetireApp, handleRetireApp)
}

func handleRetireApp(v runtime.View, s Model, p RetireAppPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	i := s.findName(p.Name)
	if i < 0 || s.Apps[i].Status != StatusRemoving {
		return s, nil
	}

	return s, []runtime.Cmd{
		NewRemoveAppUnit(RemoveAppUnitPayload{
			Name: s.Apps[i].Name,
		}),
	}
}
