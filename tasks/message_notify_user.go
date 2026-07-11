package tasks

import (
	emailmsg "github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/tasks/msg"
)

// "notify-user" — injected by the control endpoint; email the task initiator a mid-run one-liner

func registerNotifyUser(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.NotifyUser, handleNotifyUser)
}

// handleNotifyUser turns a mid-run notification into a threaded one-line mail to
// the task's initiator. It mirrors replyCmds' derivation of the From/threading
// headers from the Task, but a notification is DELIBERATELY one-way: the task is
// not completed, no reply is marked, and the model is not mutated — s is returned
// unchanged (feature-notifications.md). The send-notification command fires the
// mail fire-and-forget, outside the outbox/reconcile path.
func handleNotifyUser(v runtime.View, s Model, p msg.NotifyUserPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	t, ok := Get(v, TaskID(p.TaskID))
	if !ok {
		return s, nil
	}

	from := s.ReplyFrom
	if from == "" {
		from = routeAddr(t.Route)
	}

	// meta.Offset (the notify-user message's log offset) is the per-notification
	// sequence, so multiple notifies in one run each get a unique Message-ID.
	messageID := emailmsg.NotifyMessageID(p.TaskID, meta.Offset, mime.Domain(from))

	return s, []runtime.Cmd{emailmsg.NewSendNotification(emailmsg.SendNotificationPayload{
		To:         t.Participants,
		From:       from,
		Subject:    mime.ReplySubject(t.Subject),
		InReplyTo:  t.LastMessageID,
		References: t.References,
		MessageID:  messageID,
		Body:       p.Body,
		TaskID:     p.TaskID,
		RunID:      p.RunID,
		Mechanism:  activeSender(v),
	})}
}

// outboundViaReader is the one method this handler needs from the email slice.
// Defining it here, in the consumer, lets tasks read the active sender without
// importing the email package — which imports tasks, so the dependency must stay
// one-way (decision-023).
type outboundViaReader interface{ OutboundViaName() string }

// activeSender resolves the mechanism a notification should leave through — the
// same OutboundVia the reply path uses — so a notification and a reply exit through
// the same provider. Defaults to the spool when email is unconfigured.
func activeSender(v runtime.View) string {
	if s, ok := v.Slice("email"); ok {
		if r, ok := s.(outboundViaReader); ok {
			return r.OutboundViaName()
		}
	}
	return "spool"
}
