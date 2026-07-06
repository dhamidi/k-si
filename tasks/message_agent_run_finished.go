package tasks

import (
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

	// A reply goes out only when the run actually succeeded and produced one. A
	// stopped run, a non-zero exit (the agent crashed or timed out), or a run that
	// wrote no reply.txt must NOT email the user an empty or broken message — keep
	// the transcript and hand the task back to the human (docs/05).
	if p.Stopped || p.Exit != 0 || !hasReply(p.OutManifest) {
		tasks[i] = t
		s.Tasks = tasks
		return s, []runtime.Cmd{capture}
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

	return s, []runtime.Cmd{capture, assemble}
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
