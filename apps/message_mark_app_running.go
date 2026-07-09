package apps

import "github.com/dhamidi/k-si/runtime"

// "mark-app-running" — the app's unit is installed and started; flip Status to running so reconcile leaves it alone
const MarkAppRunning = "mark-app-running"

type MarkAppRunningPayload struct {
	Name string `json:"name"`
}

func NewMarkAppRunning(p MarkAppRunningPayload) runtime.Msg {
	return runtime.NewMsg(MarkAppRunning, p)
}

func registerMarkAppRunning(mod *runtime.Module) {
	runtime.HandleMsg(mod, MarkAppRunning, handleMarkAppRunning)
}

func handleMarkAppRunning(v runtime.View, s Model, p MarkAppRunningPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	i := s.findName(p.Name)
	if i < 0 {
		return s, nil
	}

	next := append([]App(nil), s.Apps...)
	next[i].Status = StatusRunning
	s.Apps = next

	return s, nil
}
