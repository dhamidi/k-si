package msg

import "github.com/dhamidi/k-si/runtime"

// "restart-app" — bounce a running app's unit so it picks up new code; injected by the /control/app endpoint, records the intent so the effect is auditable in the log
const RestartApp = "restart-app"

type RestartAppPayload struct {
	Name string `json:"name"`
}

func NewRestartApp(p RestartAppPayload) runtime.Msg {
	return runtime.NewMsg(RestartApp, p)
}
