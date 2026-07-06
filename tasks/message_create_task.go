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
		ID:              id,
		Status:          AwaitingAgent,
		Route:           p.Route,
		Template:        p.Template,
		Subject:         p.Subject,
		Participants:    dedup(append([]string{p.Sender}, p.Cc...)),
		References:      []string{p.MessageID},
		LastMessageID:   p.MessageID,
		CompletionToken: p.CompletionToken,
		InboxIDs:        []int64{p.InboxID},
	}
	s.Tasks = append(append([]Task(nil), s.Tasks...), t)

	return s, []runtime.Cmd{
		NewLayInFromInbox(LayInFromInboxPayload{TaskID: int64(id), InboxID: p.InboxID}),
		runtime.Send(agentmsg.NewSpawnAgentRun(agentmsg.SpawnAgentRunPayload{TaskID: int64(id), Resume: false})),
	}
}
