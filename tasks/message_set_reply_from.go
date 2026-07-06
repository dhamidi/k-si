package tasks

import (
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/tasks/msg"
)

// "set-reply-from" — set the deliverable From address replies are sent as (docs/04)

func registerSetReplyFrom(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.SetReplyFrom, handleSetReplyFrom)
}

func handleSetReplyFrom(v runtime.View, s Model, p msg.SetReplyFromPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	s.ReplyFrom = p.Address
	return s, nil
}
