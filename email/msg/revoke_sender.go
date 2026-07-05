package msg

import "github.com/dhamidi/k-si/runtime"

// "revoke-sender" — remove an address from the initiator allowlist (from the web UI)
const RevokeSender = "revoke-sender"

type RevokeSenderPayload struct {
	Address string `json:"address"`
}

func NewRevokeSender(p RevokeSenderPayload) runtime.Msg {
	return runtime.NewMsg(RevokeSender, p)
}
