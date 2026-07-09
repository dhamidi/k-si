package msg

import "github.com/dhamidi/k-si/runtime"

// "set-allowlist" — replace the whole initiator allowlist from the settings UI
// (docs/16, decision-020). A whole-value REPLACE: the settings form edits the
// entire list at once, semantics the incremental allow-sender/revoke-sender
// cannot express, so those stay for CC-granting and single-address writes while
// this owns the UI edit. Both write email.Model.Allowlist.
const SetAllowlist = "set-allowlist"

type SetAllowlistPayload struct {
	Addresses []string `json:"addresses"`
}

func NewSetAllowlist(p SetAllowlistPayload) runtime.Msg {
	return runtime.NewMsg(SetAllowlist, p)
}
