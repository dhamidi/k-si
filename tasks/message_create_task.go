package tasks

import (
	agentmsg "github.com/dhamidi/k-si/agents/msg"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/tasks/msg"
)

// "create-task" — sent by email/route-email; creates the Task and seeds participants

func registerCreateTask(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.CreateTask, handleCreateTask)
}

func handleCreateTask(v runtime.View, s Model, p msg.CreateTaskPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	id := TaskID(meta.Offset)
	t := Task{
		ID:       id,
		Status:   AwaitingAgent,
		Route:    p.Route,
		Template: p.Template,
		Subject:  p.Subject,
		// Participants are everyone on the thread — sender + To + Cc — minus käsi's own
		// addresses (dropOwn), so a multi-party mail reply-alls to all of them
		// (multiplayer, decision-017) while käsi never replies to itself (SEV1,
		// decision-016). Every reply path builds recipients from Participants, so the
		// exclusion holds everywhere downstream.
		Participants:    dropOwn(dedup(append(append([]string{p.Sender}, p.To...), p.Cc...)), s.ReplyFrom),
		References:      []string{p.MessageID},
		LastMessageID:   p.MessageID,
		CompletionToken: p.CompletionToken,
		InboxIDs:        []int64{p.InboxID},
		// Turns counts agent runs spawned for this task — the loop breaker's meter
		// (decision-016). The create spawns the first run, so it starts at 1.
		Turns: 1,
	}
	s.Tasks = append(append([]Task(nil), s.Tasks...), t)

	return s, []runtime.Cmd{
		NewLayInFromInbox(LayInFromInboxPayload{TaskID: int64(id), InboxID: p.InboxID}),
		runtime.Send(agentmsg.NewSpawnAgentRun(agentmsg.SpawnAgentRunPayload{TaskID: int64(id), Resume: false})),
	}
}
