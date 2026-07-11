package msg

import "github.com/dhamidi/k-si/runtime"

// "set-mechanism" — configure one delivery mechanism (upsert its entry, keyed by name; carries secret:// credential references, never plaintext)
const SetMechanism = "set-mechanism"

type SetMechanismPayload struct {
	Name        string `json:"name"`
	Inbound     bool   `json:"inbound"`
	Outbound    bool   `json:"outbound"`
	Domain      string `json:"domain"`
	SendCredRef string `json:"send_cred_ref"`
	RecvCredRef string `json:"recv_cred_ref"`
}

func NewSetMechanism(p SetMechanismPayload) runtime.Msg {
	return runtime.NewMsg(SetMechanism, p)
}
