package apps

import "github.com/dhamidi/k-si/runtime"

// "mark-app-removed" — the app's unit is stopped and gone; drop its entry from the registry
const MarkAppRemoved = "mark-app-removed"

type MarkAppRemovedPayload struct {
	Name string `json:"name"`
}

func NewMarkAppRemoved(p MarkAppRemovedPayload) runtime.Msg {
	return runtime.NewMsg(MarkAppRemoved, p)
}

func registerMarkAppRemoved(mod *runtime.Module) {
	runtime.HandleMsg(mod, MarkAppRemoved, handleMarkAppRemoved)
}

func handleMarkAppRemoved(v runtime.View, s Model, p MarkAppRemovedPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	i := s.findName(p.Name)
	if i < 0 {
		return s, nil
	}

	next := append([]App(nil), s.Apps[:i]...)
	next = append(next, s.Apps[i+1:]...)
	s.Apps = next

	return s, nil
}
