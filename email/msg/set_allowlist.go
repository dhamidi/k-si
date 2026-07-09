package msg

import "github.com/dhamidi/k-si/runtime"

// "set-allowlist" — replace the whole initiator allowlist (the settings-UI whole-value write; incremental allow-sender/revoke-sender stay)
const SetAllowlist = "set-allowlist"

type SetAllowlistPayload struct {
	Addresses []string `json:"addresses"`
}

func NewSetAllowlist(p SetAllowlistPayload) runtime.Msg {
	return runtime.NewMsg(SetAllowlist, p)
}
