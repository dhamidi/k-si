package msg

import "github.com/dhamidi/k-si/runtime"

// "set-outbound-via" — choose the mechanism that sends käsi's replies (the single active sender)
const SetOutboundVia = "set-outbound-via"

type SetOutboundViaPayload struct {
	Name string `json:"name"`
}

func NewSetOutboundVia(p SetOutboundViaPayload) runtime.Msg {
	return runtime.NewMsg(SetOutboundVia, p)
}
