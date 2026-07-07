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

	// The mint completed: clear the request HarvestJob agent-run-finished owed for
	// this run. Clearing it HERE, atomically with recording the UIRequest, makes
	// marker-present ⟺ not-yet-registered — there is no partial-emit window, so the
	// mint's reconcile source re-drives only when nothing was ever registered, and no
	// dedup is needed (decision-013). An absent marker (a re-fold, or a non-reconciled
	// caller) is a harmless no-op.
	s.HarvestPending = withoutHarvestPending(s.HarvestPending, p.RunID, HarvestRequest)

	// The reply carrying the request link goes out via the reconciled reply path, not
	// an inline assemble-reply — a crash between logging this register-ui-request and
	// the reply finishing would otherwise lose the request's reply (replay re-derives
	// the Cmd but SUPPRESSES effects). replyCmds reconstructs the AssembleReplyPayload
	// from the logged Task and derives RequestLink from the UIRequest recorded just
	// above; assemble-reply ends with mark-harvested{reply}, which clears this job.
	s.HarvestPending = withHarvestPending(s.HarvestPending, HarvestJob{TaskID: p.TaskID, RunID: p.RunID, Kind: HarvestReply})

	return s, nil
}
