package msg

import "github.com/dhamidi/k-si/runtime"

// "forget" — remove a memory by name (idempotent)
const Forget = "forget"

type ForgetPayload struct {
	Name string `json:"name"`
}

func NewForget(p ForgetPayload) runtime.Msg {
	return runtime.NewMsg(Forget, p)
}
