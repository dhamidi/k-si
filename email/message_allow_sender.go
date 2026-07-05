package email

import (
	"github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "allow-sender" — add an address to the initiator allowlist (from the web UI)

func registerAllowSender(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.AllowSender, handleAllowSender)
}

func handleAllowSender(v runtime.View, s Model, p msg.AllowSenderPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	return s, nil
}
