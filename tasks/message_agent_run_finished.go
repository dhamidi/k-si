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

	// A stopped run yields no reply — the human already has the thread, we only
	// keep the transcript (docs/05).
	if p.Stopped {
		tasks[i] = t
		s.Tasks = tasks
		return s, []runtime.Cmd{capture}
	}

	// Record the reply's deterministic Message-ID in References BEFORE the user
	// can reply to it, so the next inbound threads back onto this task.
	replyID := emailmsg.ReplyMessageID(p.TaskID, p.RunID)
	t.References = append(append([]string(nil), t.References...), replyID)
	tasks[i] = t
	s.Tasks = tasks

	assemble := emailmsg.NewAssembleReply(emailmsg.AssembleReplyPayload{
		TaskID:          p.TaskID,
		RunID:           p.RunID,
		From:            routeAddr(t.Route),
		To:              t.Participants,
		Subject:         mime.ReplySubject(t.Subject),
		InReplyTo:       t.LastMessageID,
		References:      t.References,
		CompletionToken: t.CompletionToken,
		OutManifest:     p.OutManifest,
	})

	return s, []runtime.Cmd{capture, assemble}
}
