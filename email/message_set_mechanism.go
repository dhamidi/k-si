package email

import (
	"github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "set-mechanism" — configure one delivery mechanism (upsert its entry, keyed by name; carries secret:// credential references, never plaintext)

func registerSetMechanism(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.SetMechanism, handleSetMechanism)
}

func handleSetMechanism(v runtime.View, s Model, p msg.SetMechanismPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// Upsert ONE mechanism's config, keyed by name — not a whole-map replace — so
	// mechanisms configured independently never clobber each other (decision-023).
	// Copy-on-write: a fresh map (or a copy of the existing one) keeps this handler
	// pure and the model slice's own map unshared. The payload carries only
	// secret:// references in SendCredRef/RecvCredRef; plaintext never reaches the
	// model or log (decision-004). No commands — this is pure recorded state.
	next := make(map[string]Mechanism, len(s.Mechanisms)+1)
	for k, m := range s.Mechanisms {
		next[k] = m
	}
	next[p.Name] = Mechanism{
		Inbound:     p.Inbound,
		Outbound:    p.Outbound,
		Domain:      p.Domain,
		SendCredRef: p.SendCredRef,
		RecvCredRef: p.RecvCredRef,
	}
	s.Mechanisms = next
	return s, nil
}
