package email

import (
	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/tasks"
	taskmsg "github.com/dhamidi/k-si/tasks/msg"
)

// "route-email" — announced for each stored inbound mail; authorise the sender, resolve the route, hand off to tasks
const RouteEmail = "route-email"

type RouteEmailPayload struct {
	InboxID    int64    `json:"inbox_id"`
	Recipient  string   `json:"recipient"`
	Sender     string   `json:"sender"`
	Cc         []string `json:"cc"`
	Subject    string   `json:"subject"`
	MessageID  string   `json:"message_id"`
	InReplyTo  string   `json:"in_reply_to"`
	References []string `json:"references"`
	// CompletionToken is minted at the inbound edge (crypto/rand in production,
	// deterministic in the sim ring) and carried to the task it creates, so the
	// completion link is unguessable and randomness never enters a pure handler
	// (docs/04, docs/13).
	CompletionToken string `json:"completion_token"`
}

func NewRouteEmail(p RouteEmailPayload) runtime.Msg {
	return runtime.NewMsg(RouteEmail, p)
}

func registerRouteEmail(mod *runtime.Module) {
	runtime.HandleMsg(mod, RouteEmail, handleRouteEmail)
}

func handleRouteEmail(v runtime.View, s Model, p RouteEmailPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// käsi never acts on its own mail. A reply käsi sent (To/Cc'ing itself) lands
	// back in its own mailbox and the poller re-ingests it as inbound; routing it in
	// would thread it onto the task and spawn another run, which replies again — the
	// self-reply loop that OOM-killed the box (SEV1, decision-016). Drop it here, at
	// the earliest choke point, before create OR append. Self is käsi's own
	// deliverable identity (tasks.ReplyFrom); SameAddress treats an unset identity
	// (the sim ring) as "no self", so scenarios are unaffected.
	if mime.SameAddress(p.Sender, tasks.ReplyFrom(v)) {
		return s, nil
	}

	// A reply threads onto an existing task if its In-Reply-To / References match
	// that task's thread — and only a participant of THAT task may act on it
	// (docs/04). Whether the sender is a participant is tasks' state, read through
	// tasks' own helper (a one-directional cross-domain read, docs/15).
	if id, ok := tasks.ByThreadKey(v, p.InReplyTo, p.References); ok {
		if t, ok := tasks.Get(v, id); ok && tasks.IsParticipant(t, p.Sender) {
			return s, []runtime.Cmd{runtime.Send(taskmsg.NewAppendToTask(taskmsg.AppendToTaskPayload{
				TaskID:    int64(id),
				InboxID:   p.InboxID,
				Sender:    p.Sender,
				Cc:        p.Cc,
				Subject:   p.Subject,
				MessageID: p.MessageID,
				InReplyTo: p.InReplyTo,
			}))}
		}
		// A reply from a non-participant is ignored for that task (docs/04).
		return s, nil
	}

	// A new task: the sender must be on the initiator allowlist — käsi's spam
	// boundary. Unauthorised mail is stored and ignored, never an error (docs/04).
	if !s.allows(p.Sender) {
		return s, nil
	}

	route, template := routeFor(mime.LocalPart(p.Recipient))
	return s, []runtime.Cmd{runtime.Send(taskmsg.NewCreateTask(taskmsg.CreateTaskPayload{
		InboxID:         p.InboxID,
		Route:           route,
		Template:        template,
		Sender:          p.Sender,
		Cc:              p.Cc,
		Subject:         p.Subject,
		MessageID:       p.MessageID,
		CompletionToken: p.CompletionToken,
	}))}
}
