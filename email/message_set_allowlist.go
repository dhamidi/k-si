package email

import (
	"sort"

	"github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "set-allowlist" — replace the whole initiator allowlist (the settings-UI whole-value write; incremental allow-sender/revoke-sender stay)

func registerSetAllowlist(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.SetAllowlist, handleSetAllowlist)
}

func handleSetAllowlist(v runtime.View, s Model, p msg.SetAllowlistPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// Whole-value replace, sorted and de-duplicated to match the invariant the
	// incremental allow-sender keeps (withAllowed). Copy-on-write; an empty payload
	// clears the list. This is the UI-replace half of the split allow-sender uses.
	seen := map[string]bool{}
	next := make([]string, 0, len(p.Addresses))
	for _, a := range p.Addresses {
		if a == "" || seen[a] {
			continue
		}
		seen[a] = true
		next = append(next, a)
	}
	sort.Strings(next)
	s.Allowlist = next
	return s, nil
}
