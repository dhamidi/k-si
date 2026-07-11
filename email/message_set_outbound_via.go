package email

import (
	"github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "set-outbound-via" — choose the mechanism that sends käsi's replies (the single active sender)

func registerSetOutboundVia(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.SetOutboundVia, handleSetOutboundVia)
}

func handleSetOutboundVia(v runtime.View, s Model, p msg.SetOutboundViaPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// Record the single active sender. Resolving it live in the send-outbox handler
	// (not here, and not at reconcile-sub build time) is what makes a change take
	// effect on the next queued reply (decision-023). No commands — pure state.
	s.OutboundVia = p.Name
	return s, nil
}
