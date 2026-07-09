package msg

import "github.com/dhamidi/k-si/runtime"

// "register-app" — record a registered app (add, or replace-in-place on re-add); keyed by name like remember updates a memory
const RegisterApp = "register-app"

type RegisterAppPayload struct {
	Name       string `json:"name"`
	Port       int    `json:"port"`
	StartCmd   string `json:"start_cmd"`
	Operations string `json:"operations"`
	URL        string `json:"url"`
}

func NewRegisterApp(p RegisterAppPayload) runtime.Msg {
	return runtime.NewMsg(RegisterApp, p)
}
