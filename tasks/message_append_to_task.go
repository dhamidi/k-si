package tasks

import (
	agentmsg "github.com/dhamidi/k-si/agents/msg"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/tasks/msg"
)

// "append-to-task" — sent by email/route-email; a participant's reply threads onto an existing task

func registerAppendToTask(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.AppendToTask, handleAppendToTask)
}

func handleAppendToTask(v runtime.View, s Model, p msg.AppendToTaskPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	i := s.find(TaskID(p.TaskID))
	if i < 0 {
		return s, nil
	}

	tasks := append([]Task(nil), s.Tasks...)
	t := tasks[i]

	// A paused task never spawns again — the loop breaker already tripped, so any
	// further threaded mail (e.g. an auto-responder still replying) is recorded but
	// drives no run (SEV1, decision-016).
	if t.Status == Paused {
		return s, nil
	}

	// The loop breaker (decision-016): a task that spawns more runs than LoopGuard
	// allows without resolving is pausing on a probable reply loop. Self-CC is
	// already dead (Fix A/B), but a benign back-and-forth with an allowlisted
	// AUTO-RESPONDER is a real loop these two guards can't see — the sender is a real
	// human address — so this caps ANY task's runs. LoopGuard 0 is off (the sim
	// default); the check trips at spawn time so the extra process never starts.
	if s.LoopGuard > 0 && t.Turns >= s.LoopGuard {
		t.Status = Paused
		t.References = append(append([]string(nil), t.References...), p.MessageID)
		t.LastMessageID = p.MessageID
		t.InboxIDs = append(append([]int64(nil), t.InboxIDs...), p.InboxID)
		tasks[i] = t
		s.Tasks = tasks
		return s, nil
	}

	incoming := append(append([]string{p.Sender}, p.To...), p.Cc...)
	t.Participants = dropOwn(dedup(append(append([]string(nil), t.Participants...), incoming...)), s.ReplyFrom)
	t.References = append(append([]string(nil), t.References...), p.MessageID)
	t.LastMessageID = p.MessageID
	t.InboxIDs = append(append([]int64(nil), t.InboxIDs...), p.InboxID)
	t.Status = AwaitingAgent
	t.Turns++
	tasks[i] = t
	s.Tasks = tasks

	return s, []runtime.Cmd{
		NewLayInFromInbox(LayInFromInboxPayload{TaskID: p.TaskID, InboxID: p.InboxID}),
		runtime.Send(agentmsg.NewSpawnAgentRun(agentmsg.SpawnAgentRunPayload{TaskID: p.TaskID, Resume: true})),
	}
}
