package msg

import "github.com/dhamidi/k-si/runtime"

// "unregister-app" — mark an app for removal (rm); the unit is torn down by reconciliation, then the entry is dropped
const UnregisterApp = "unregister-app"

type UnregisterAppPayload struct {
	Name string `json:"name"`
}

func NewUnregisterApp(p UnregisterAppPayload) runtime.Msg {
	return runtime.NewMsg(UnregisterApp, p)
}
