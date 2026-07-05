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
	t.Participants = dedup(append(append([]string(nil), t.Participants...), append([]string{p.Sender}, p.Cc...)...))
	t.References = append(append([]string(nil), t.References...), p.MessageID)
	t.LastMessageID = p.MessageID
	t.InboxIDs = append(append([]int64(nil), t.InboxIDs...), p.InboxID)
	t.Status = AwaitingAgent
	tasks[i] = t
	s.Tasks = tasks

	return s, []runtime.Cmd{
		NewLayInFromInbox(LayInFromInboxPayload{TaskID: p.TaskID, InboxID: p.InboxID}),
		runtime.Send(agentmsg.NewSpawnAgentRun(agentmsg.SpawnAgentRunPayload{TaskID: p.TaskID, Resume: true})),
	}
}
