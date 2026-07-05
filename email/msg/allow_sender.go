package msg

import "github.com/dhamidi/k-si/runtime"

// "allow-sender" — add an address to the initiator allowlist (from the web UI)
const AllowSender = "allow-sender"

type AllowSenderPayload struct {
	Address string `json:"address"`
}

func NewAllowSender(p AllowSenderPayload) runtime.Msg {
	return runtime.NewMsg(AllowSender, p)
}
