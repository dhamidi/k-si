package email

import (
	"github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "revoke-sender" — remove an address from the initiator allowlist (from the web UI)

func registerRevokeSender(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.RevokeSender, handleRevokeSender)
}

func handleRevokeSender(v runtime.View, s Model, p msg.RevokeSenderPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	return s, nil
}
