package msg

import "github.com/dhamidi/k-si/runtime"

// "set-reply-from" — set the deliverable From address replies are sent as (docs/04)
const SetReplyFrom = "set-reply-from"

type SetReplyFromPayload struct {
	Address string `json:"address"`
}

func NewSetReplyFrom(p SetReplyFromPayload) runtime.Msg {
	return runtime.NewMsg(SetReplyFrom, p)
}
