package tasks

import (
	emailmsg "github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/tasks/msg"
)

// "register-ui-request" — sent by email/mint-ui-request; record the request on
// the model, set the raising task awaiting-user, and drive the reply that
// carries the request link (Flow C, decision-002).

func registerRegisterUIRequest(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.RegisterUIRequest, handleRegisterUIRequest)
}

func handleRegisterUIRequest(v runtime.View, s Model, p msg.RegisterUIRequestPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	i := s.find(TaskID(p.TaskID))
	if i < 0 {
		return s, nil
	}

	// Record the pending request (copy-on-write).
	s.Requests = append(append([]UIRequest(nil), s.Requests...), UIRequest{
		RunID:    p.RunID,
		TaskID:   p.TaskID,
		Token:    p.Token,
		FormSpec: p.FormSpec,
		Link:     p.Link,
		Status:   RequestPending,
	})

	tasks := append([]Task(nil), s.Tasks...)
	t := tasks[i]
	t.Status = AwaitingUser

	// The reply goes out as the configured deliverable identity, falling back to
	// the routeAddr placeholder (the sim ring never sends). Mirror the normal
	// finished-success path: record the deterministic reply Message-ID in
	// References BEFORE the user can reply, so their next inbound threads back.
	from := s.ReplyFrom
	if from == "" {
		from = routeAddr(t.Route)
	}
	replyID := emailmsg.ReplyMessageID(p.TaskID, p.RunID, mime.Domain(from))
	t.References = append(append([]string(nil), t.References...), replyID)
	tasks[i] = t
	s.Tasks = tasks

	// The ONLY difference from a normal reply is RequestLink: assemble-reply
	// harvests out/ for the body (reply.txt) itself, so no OutManifest is needed.
	assemble := emailmsg.NewAssembleReply(emailmsg.AssembleReplyPayload{
		TaskID:          p.TaskID,
		RunID:           p.RunID,
		From:            from,
		To:              t.Participants,
		Subject:         mime.ReplySubject(t.Subject),
		InReplyTo:       t.LastMessageID,
		References:      t.References,
		CompletionToken: t.CompletionToken,
		RequestLink:     p.Link,
	})

	return s, []runtime.Cmd{assemble}
}
