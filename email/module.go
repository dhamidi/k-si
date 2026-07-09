package email

import (
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/store"
	"github.com/dhamidi/k-si/workspace"
)

// Edges is everything email touches in the world. Real implementations are
// wired in cmd/kasi/main.go; simulated twins live in this package (docs/12).
type Edges struct {
	Clock   runtime.Clock
	Mail    Mail
	Content store.Content
	Work    workspace.Workspace
}

// Module bundles Fastmail routing, the initiator allowlist, the inbox/outbox, and reply assembly (docs/01).
func Module(e Edges) *runtime.Module {
	mod := runtime.NewModule("email", Model{}, e)

	registerRouteEmail(mod)
	registerAllowSender(mod)
	registerRevokeSender(mod)
	registerMarkReplyQueued(mod)
	registerMarkEmailSent(mod)
	registerAssembleReply(mod)
	registerMintUIRequest(mod)
	registerSendEmail(mod)
	runtime.Subscribe(mod, outboxReconcileSubs)
	registerSendOutbox(mod)
	registerSendNotification(mod)
	registerRecordPollState(mod)
	registerSetAllowlist(mod)
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
		Mail:    NewSimMail(content),
		Content: content,
		Work:    workspace.NewMemory(),
	}
}
