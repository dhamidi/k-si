package email

import (
	"sort"

	"github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "set-allowlist" — replace the whole initiator allowlist (from the settings UI)

func registerSetAllowlist(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.SetAllowlist, handleSetAllowlist)
}

func handleSetAllowlist(v runtime.View, s Model, p msg.SetAllowlistPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// Whole-value replace, sorted so the model marshals stably for the
	// replay-convergence check (docs/13) — the same deterministic order
	// withAllowed keeps. The addresses were already validated at the web edge
	// (each row parses through addressValue), so the reducer just records them.
	next := append([]string(nil), p.Addresses...)
	sort.Strings(next)
	s.Allowlist = next
	return s, nil
}
