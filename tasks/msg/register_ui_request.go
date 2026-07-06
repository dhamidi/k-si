package msg

import "github.com/dhamidi/k-si/runtime"

// "register-ui-request" — sent by email/mint-ui-request; records the agent's UI
// request on the tasks model and drives the reply that carries its link.
const RegisterUIRequest = "register-ui-request"

type RegisterUIRequestPayload struct {
	TaskID   int64  `json:"task_id"`
	RunID    int64  `json:"run_id"`
	Token    string `json:"token"`
	FormSpec []byte `json:"form_spec"`
	Link     string `json:"link"`
}

func NewRegisterUIRequest(p RegisterUIRequestPayload) runtime.Msg {
	return runtime.NewMsg(RegisterUIRequest, p)
}
