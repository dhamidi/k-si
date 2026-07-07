package tasks

import (
	"strings"

	emailmsg "github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/tasks/msg"
)

// "agent-run-finished" — sent by agents when a run exits; harvest out/, capture the transcript, reply

func registerAgentRunFinished(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.AgentRunFinished, handleAgentRunFinished)
}

func handleAgentRunFinished(v runtime.View, s Model, p msg.AgentRunFinishedPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	i := s.find(TaskID(p.TaskID))
	if i < 0 {
		return s, nil
	}

	tasks := append([]Task(nil), s.Tasks...)
	t := tasks[i]
	t.Runs = append(append([]int64(nil), t.Runs...), p.RunID)
	t.Status = AwaitingUser

	capture := NewCaptureTranscript(CaptureTranscriptPayload{
		TaskID:         p.TaskID,
		RunID:          p.RunID,
		TranscriptPath: p.TranscriptPath,
	})

	// A stopped or failed run (crash/timeout) yields nothing to send — keep the
	// transcript and hand the task back to the human (docs/05). This gate runs
	// before the request, reply, and skill branches, so a crash never mints a
	// request, emails a reply, nor stores a skill, whatever half-written files it
	// left in out/.
	if p.Stopped || p.Exit != 0 {
		tasks[i] = t
		s.Tasks = tasks
		return s, []runtime.Cmd{capture}
	}

	// A successful run may ADDITIVELY author one or more skills (Flow D,
	// decision-009): it wrote out/skills/<name>/SKILL.md. This is orthogonal to
	// the reply/request branches — a run may author a skill and reply, and raise a
	// request, or author with none of those — so store-skill rides alongside
	// whatever else the run produced. capture stays first; store-skill next.
	cmds := []runtime.Cmd{capture}
	if hasSkill(p.OutManifest) {
		cmds = append(cmds, NewStoreSkill(StoreSkillPayload{TaskID: p.TaskID, RunID: p.RunID}))
	}

	// Harvest memory on EVERY successful finish (feature-memory.md): out/memory/
	// writes become remember directives, and an in/memory/ deletion becomes a forget
	// — the deletion leaves no out/ artifact, so this cannot be gated on a manifest
	// marker the way store-skill is.
	//
	// Do NOT emit capture-memory inline here. That is a Cmd/effect, and a crash
	// between logging this agent-run-finished and the effect finishing would lose
	// the harvest forever: restart→replay re-derives the capture-memory Cmd but
	// replay suppresses effects, and the remember/forget were never logged. Instead
	// record the harvest as PENDING WORK on the model (copy-on-write). The
	// harvest-reconcile subscription turns each pending job into the capture-memory
	// effect; the effect ends by emitting mark-harvested, which clears the job. A
	// crash anywhere before mark-harvested leaves the job pending, and restart's
	// replay rebuilds it so the source fires again — exactly the guarantee email's
	// pending outbox gives an unsent reply (docs/03).
	//
	// NOTE: store-skill (above) and assemble-reply (below) are still emitted inline
	// and share this SAME pre-existing crash-safety gap — a crash between logging
	// agent-run-finished and those effects finishing loses them. The mechanism added
	// here should be extended to cover them (supervisor to log a decision).
	s.HarvestPending = withHarvestPending(s.HarvestPending, HarvestJob{TaskID: p.TaskID, RunID: p.RunID})

	// A raised UI request takes precedence over reply.txt (Flow C): the run wrote
	// out/request.json to ask the human for input via the web. reply.txt is
	// OPTIONAL here (docs/05), so this must precede the no-reply gate below or a
	// request-only run would be dropped. Hand off to email's mint-ui-request, which
	// mints the token and link and emits register-ui-request — that handler drives
	// the reply carrying the link. No normal reply here.
	if hasRequest(p.OutManifest) {
		tasks[i] = t
		s.Tasks = tasks
		return s, append(cmds, emailmsg.NewMintUIRequest(emailmsg.MintUIRequestPayload{
			TaskID: p.TaskID,
			RunID:  p.RunID,
		}))
	}

	// A successful run that wrote no reply.txt produced nothing to send — no empty
	// email; keep the transcript (and any authored skill) and hand the task back
	// (docs/05).
	if !hasReply(p.OutManifest) {
		tasks[i] = t
		s.Tasks = tasks
		return s, cmds
	}

	// The reply is sent as the configured deliverable identity; it falls back to
	// the routeAddr placeholder only when unset (the sim ring never sends). The
	// Message-ID takes that address's domain so it matches what email stamps.
	from := s.ReplyFrom
	if from == "" {
		from = routeAddr(t.Route)
	}

	// Record the reply's deterministic Message-ID in References BEFORE the user
	// can reply to it, so the next inbound threads back onto this task.
	replyID := emailmsg.ReplyMessageID(p.TaskID, p.RunID, mime.Domain(from))
	t.References = append(append([]string(nil), t.References...), replyID)
	tasks[i] = t
	s.Tasks = tasks

	assemble := emailmsg.NewAssembleReply(emailmsg.AssembleReplyPayload{
		TaskID:          p.TaskID,
		RunID:           p.RunID,
		From:            from,
		To:              t.Participants,
		Subject:         mime.ReplySubject(t.Subject),
		InReplyTo:       t.LastMessageID,
		References:      t.References,
		CompletionToken: t.CompletionToken,
		OutManifest:     p.OutManifest,
	})

	return s, append(cmds, assemble)
}

// hasReply reports whether the agent left the reply body käsi sends. Its absence
// means a failed or misbehaving run, not an intentional silence — the worker
// prompt (docs/05) tells the agent to always write reply.txt, even to ask.
func hasReply(manifest []string) bool {
	for _, name := range manifest {
		if name == "reply.txt" {
			return true
		}
	}
	return false
}

// hasRequest reports whether the run raised a UI request — it wrote
// out/request.json, the spec the web edge renders a form from (Flow C, docs/05).
func hasRequest(manifest []string) bool {
	for _, name := range manifest {
		if name == "request.json" {
			return true
		}
	}
	return false
}

// hasSkill reports whether the run authored at least one Agent Skill — the
// (recursive) out-manifest holds a skills/<name>/SKILL.md, the required file of
// an Agent Skills directory (Flow D, decision-009). Only a SKILL.md marks a
// valid skill folder; store-skill groups the whole tree by <name>.
func hasSkill(manifest []string) bool {
	for _, name := range manifest {
		segs := strings.Split(name, "/")
		if len(segs) == 3 && segs[0] == "skills" && segs[1] != "" && segs[2] == "SKILL.md" {
			return true
		}
	}
	return false
}
