package email

import (
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/store"
	"github.com/dhamidi/k-si/workspace"
)

// Edges is everything email touches in the world. Real implementations are
// wired in cmd/kasi/main.go; simulated twins live in this package (docs/12).
type Edges struct {
	Clock runtime.Clock
	// Senders is one outbound Mail per delivery mechanism, keyed by name (spool,
	// fastmail, forwardemail, …), each built at boot with its own credential
	// reference (decision-023). The send-email effect dispatches through it by the
	// mechanism name the send-outbox handler resolved from the live model, so the
	// effect stays model-blind. An absent key leaves the outbox row pending rather
	// than dropping it.
	Senders map[string]Mail
	Content store.Content
	Work    workspace.Workspace
}

// Module bundles Fastmail routing, the initiator allowlist, the inbox/outbox, and reply assembly (docs/01).
func Module(e Edges) *runtime.Module {
	mod := runtime.NewModule("email", Model{}, e)

	registerRouteEmail(mod)
	registerAllowSender(mod)
	registerRevokeSender(mod)
	registerSetAllowlist(mod)
	registerMarkReplyQueued(mod)
	registerMarkEmailSent(mod)
	registerAssembleReply(mod)
	registerMintUIRequest(mod)
	registerSendEmail(mod)
	runtime.Subscribe(mod, outboxReconcileSubs)
	registerSendOutbox(mod)
	registerSendNotification(mod)
	registerRecordPollState(mod)
	registerSetMechanism(mod)
	registerSetOutboundVia(mod)
	return mod
}

// SimEdges is the full simulated set — what `kasi test` assembles by default,
// and the simulated twin the twin rule demands (docs/12). The mail, content, and
// workspace edges are wired to a fresh, private sim set; a real scenario run has
// the runner inject a SHARED set across modules, so this fresh one only backs the
// replay/cassette paths that never drive an effect.
func SimEdges() Edges {
	content := store.NewMemoryContent()
	return Edges{
		Clock:   runtime.SimClock(),
		Senders: SimSenders(NewSimMail(content)),
		Content: content,
		Work:    workspace.NewMemory(),
	}
}

// SimSenders maps every mechanism the real boot builds onto a single observable
// Mail twin, so a scenario that selects any configured mechanism still routes to
// the twin the `outbound` vocab observes — while an unknown/unconfigured mechanism
// stays absent, so an outbox row addressed to it is left pending, not dropped
// (decision-023). Distinguishing backends by more than "something was sent" would
// need per-name sim twins, deliberately deferred.
func SimSenders(m Mail) map[string]Mail {
	return map[string]Mail{"spool": m, "fastmail": m, "forwardemail": m}
}
