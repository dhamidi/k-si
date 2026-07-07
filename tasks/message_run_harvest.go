package tasks

import (
	emailmsg "github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/runtime"
)

// "run-harvest" — the harvest-reconcile subscription's trigger. A subscription
// cannot return a command (its emit takes a Msg, not a Cmd), so — exactly as
// email's send-outbox does for send-email (docs/03) — the reconcile source emits
// this message and its handler turns it into the effect for the job's KIND. One
// run-harvest per still-pending HarvestJob, until mark-harvested clears the job.
const RunHarvest = "run-harvest"

type RunHarvestPayload struct {
	TaskID int64  `json:"task_id"`
	RunID  int64  `json:"run_id"`
	Kind   string `json:"kind"`
}

func NewRunHarvest(p RunHarvestPayload) runtime.Msg {
	return runtime.NewMsg(RunHarvest, p)
}

func registerRunHarvest(mod *runtime.Module) {
	runtime.HandleMsg(mod, RunHarvest, handleRunHarvest)
}

// handleRunHarvest turns a pending job into its effect, dispatching by kind. The
// effect ends by emitting mark-harvested{RunID, Kind}, which clears the job; a
// crash before that leaves the job pending for the reconcile source to re-drive.
// It has the whole model (via s), so the reply kind reconstructs its
// assemble-reply payload from the Task exactly as agent-run-finished built it —
// the payload is a pure projection of logged model state.
func handleRunHarvest(v runtime.View, s Model, p RunHarvestPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	switch p.Kind {
	case HarvestMemory:
		return s, []runtime.Cmd{NewCaptureMemory(CaptureMemoryPayload{
			TaskID: p.TaskID,
			RunID:  p.RunID,
		})}

	case HarvestSkill:
		return s, []runtime.Cmd{NewStoreSkill(StoreSkillPayload{
			TaskID: p.TaskID,
			RunID:  p.RunID,
		})}

	case HarvestReply:
		return s, replyCmds(s, p.TaskID, p.RunID)

	case HarvestRequest:
		// The mint reads out/request.json itself (Work.Harvest), so the payload needs
		// nothing more than the run identity. mint-ui-request emits register-ui-request,
		// which records the UIRequest, clears this request job atomically, and enqueues
		// the reply job that carries the request link.
		return s, []runtime.Cmd{emailmsg.NewMintUIRequest(emailmsg.MintUIRequestPayload{
			TaskID: p.TaskID,
			RunID:  p.RunID,
		})}

	default:
		// An unknown kind is a no-op — recorded as a dead send by the runtime, never
		// silently mistaken for done. A live-authored job always carries a known kind.
		return s, nil
	}
}

// replyCmds reconstructs the AssembleReplyPayload for a run's reply harvest from
// logged model state — the same fields agent-run-finished (and
// register-ui-request) build from: the reply-from identity, the threaded
// subject/participants/references, and the completion token. The reply's
// deterministic Message-ID was already recorded in the Task's References by
// agent-run-finished, so reading them back here yields the identical payload.
//
// OutManifest is omitted: assembleReplyEffect harvests out/ itself and never reads
// it, so it is not a model field and not needed. CauseMessageID is empty for the
// normal finished-run reply, matching agent-run-finished.
//
// RequestLink distinguishes the two callers this one reconstruction now serves: a
// normal finished-run reply carries none, but a Flow C request reply (register-ui-
// request enqueues a reply job after recording the UIRequest) carries the request's
// capability link. It is derived here from the run's own recorded UIRequest — a
// still-pending request for THIS run — so replyCmds reads it back from logged model
// state exactly as it reads the threaded headers, keeping the single path replay-
// stable for both callers.
func replyCmds(s Model, taskID, runID int64) []runtime.Cmd {
	i := s.find(TaskID(taskID))
	if i < 0 {
		return nil
	}
	t := s.Tasks[i]

	from := s.ReplyFrom
	if from == "" {
		from = routeAddr(t.Route)
	}

	requestLink := ""
	if ri := s.findRequest(runID); ri >= 0 && s.Requests[ri].Status == RequestPending {
		requestLink = s.Requests[ri].Link
	}

	return []runtime.Cmd{emailmsg.NewAssembleReply(emailmsg.AssembleReplyPayload{
		TaskID:          taskID,
		RunID:           runID,
		From:            from,
		To:              t.Participants,
		Subject:         mime.ReplySubject(t.Subject),
		InReplyTo:       t.LastMessageID,
		References:      t.References,
		CompletionToken: t.CompletionToken,
		RequestLink:     requestLink,
	})}
}
