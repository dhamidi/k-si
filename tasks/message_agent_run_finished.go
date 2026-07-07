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

	// The transcript is harvested like the rest of the fan-out: record a KIND-tagged
	// PENDING job instead of firing capture-transcript inline. EVERY finished run —
	// successful, stopped, or failed — owes a transcript, so this is unconditional
	// and precedes the stopped/failed gate. It was the last fire-and-forget effect:
	// an inline capture lost on a crash between logging this message and the archive
	// write was never re-driven (replay SUPPRESSES effects), and archive-task's
	// transcript special-case skips it at finish, so the transcript was gone forever.
	// AddArchive is now idempotent (content-addressed), so a re-driven capture never
	// duplicates the row — the property that finally makes this reconcilable
	// (decision-013).
	s.HarvestPending = withHarvestPending(s.HarvestPending, HarvestJob{TaskID: p.TaskID, RunID: p.RunID, Kind: HarvestTranscript})

	// A stopped or failed run (crash/timeout) yields nothing to send — keep the
	// transcript (via its pending job) and hand the task back to the human (docs/05).
	// This gate runs before the request, reply, and skill branches, so a crash never
	// mints a request, emails a reply, nor stores a skill, whatever half-written files
	// it left in out/.
	if p.Stopped || p.Exit != 0 {
		tasks[i] = t
		s.Tasks = tasks
		return s, nil
	}

	// The post-finish fan-out — capture-transcript, store-skill, capture-memory,
	// assemble-reply — is durable work that MUST survive a restart, so NONE of it is
	// emitted as an inline Cmd here. An inline effect is lost on a crash between
	// logging this agent-run-finished and the effect finishing: restart→replay
	// re-derives the Cmd but replay SUPPRESSES effects, and the messages it would have
	// emitted were never logged (decision-013). Instead each is recorded as a
	// KIND-tagged PENDING job on the model (copy-on-write); the harvest-reconcile
	// subscription turns each job into its effect, and the effect ends by emitting
	// mark-harvested{Kind}, which clears the job. A crash before that leaves the job
	// pending, and restart's replay rebuilds it so the source fires again — the
	// guarantee email's pending outbox gives an unsent reply (docs/03).

	// A successful run may ADDITIVELY author one or more skills (Flow D,
	// decision-009): it wrote out/skills/<name>/SKILL.md. Orthogonal to the
	// reply/request branches, so the skill job rides alongside whatever else the run
	// produced. store-skill is idempotent (AddSkill upsert-by-name + register-skill
	// upsert), so re-driving it is safe.
	if hasSkill(p.OutManifest) {
		s.HarvestPending = withHarvestPending(s.HarvestPending, HarvestJob{TaskID: p.TaskID, RunID: p.RunID, Kind: HarvestSkill})
	}

	// Harvest memory on EVERY successful finish (feature-memory.md): out/memory/
	// writes become remember directives, and an in/memory/ deletion becomes a forget
	// — the deletion leaves no out/ artifact, so this cannot be gated on a manifest
	// marker the way store-skill is.
	s.HarvestPending = withHarvestPending(s.HarvestPending, HarvestJob{TaskID: p.TaskID, RunID: p.RunID, Kind: HarvestMemory})

	// A raised UI request takes precedence over reply.txt (Flow C): the run wrote
	// out/request.json to ask the human for input via the web. reply.txt is
	// OPTIONAL here (docs/05), so this must precede the no-reply gate below or a
	// request-only run would be dropped. The mint is reconciled like the rest of the
	// fan-out: record a KIND-tagged PENDING request job instead of firing
	// mint-ui-request inline. A crash between logging this agent-run-finished and the
	// mint emitting register-ui-request would otherwise lose the whole request
	// (replay re-derives the Cmd but SUPPRESSES effects, and register-ui-request was
	// never logged). The reconcile source drives run-harvest{request}, whose handler
	// emits mint-ui-request; register-ui-request clears the marker atomically as it
	// records the UIRequest, so marker-present ⟺ not-yet-registered and re-drive only
	// happens when nothing was registered. The crypto/rand token is not a blocker: a
	// re-drive mints a FRESH token only when the prior mint never completed, and that
	// token rides register-ui-request into the log, replay-stable (decision-013).
	if hasRequest(p.OutManifest) {
		tasks[i] = t
		s.Tasks = tasks
		s.HarvestPending = withHarvestPending(s.HarvestPending, HarvestJob{TaskID: p.TaskID, RunID: p.RunID, Kind: HarvestRequest})
		return s, nil
	}

	// A successful run that wrote no reply.txt produced nothing to send — no empty
	// email; keep the transcript (and any authored skill) and hand the task back
	// (docs/05).
	if !hasReply(p.OutManifest) {
		tasks[i] = t
		s.Tasks = tasks
		return s, nil
	}

	// The reply is sent as the configured deliverable identity; it falls back to
	// the routeAddr placeholder only when unset (the sim ring never sends). The
	// Message-ID takes that address's domain so it matches what email stamps.
	from := s.ReplyFrom
	if from == "" {
		from = routeAddr(t.Route)
	}

	// Record the reply's deterministic Message-ID in References BEFORE the user
	// can reply to it, so the next inbound threads back onto this task. This is pure
	// model state and stays here even though the assemble effect is deferred — the
	// reply harvest reconstructs its payload FROM these References (see replyCmds).
	replyID := emailmsg.ReplyMessageID(p.TaskID, p.RunID, mime.Domain(from))
	t.References = append(append([]string(nil), t.References...), replyID)
	tasks[i] = t
	s.Tasks = tasks

	// Defer assemble-reply to a reply harvest job. assemble-reply is now idempotent
	// (deterministic Message-ID + AddOutbox idempotent on it), so re-driving it never
	// queues a second reply.
	s.HarvestPending = withHarvestPending(s.HarvestPending, HarvestJob{TaskID: p.TaskID, RunID: p.RunID, Kind: HarvestReply})

	return s, nil
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
