package tasks

import (
	agentmsg "github.com/dhamidi/k-si/agents/msg"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/tasks/msg"
)

// "answer-ui-request" — sent by the web edge; mark the request answered and
// resume the task: lay the collected inputs into the workspace, then spawn a
// resume run carrying the secret references (Flow C, decision-002/004).

func registerAnswerUIRequest(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.AnswerUIRequest, handleAnswerUIRequest)
}

func handleAnswerUIRequest(v runtime.View, s Model, p msg.AnswerUIRequestPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	i := s.findRequest(p.RunID)
	if i < 0 {
		return s, nil
	}

	// Mark the request answered, carrying only references (copy-on-write).
	requests := append([]UIRequest(nil), s.Requests...)
	r := requests[i]
	r.Status = RequestAnswered
	r.Values = p.Values
	r.FileRefs = p.FileRefs
	r.SecretRefs = p.SecretRefs
	requests[i] = r
	s.Requests = requests

	// Hand the task back to the agent (copy-on-write).
	if ti := s.find(TaskID(p.TaskID)); ti >= 0 {
		tasks := append([]Task(nil), s.Tasks...)
		t := tasks[ti]
		t.Status = AwaitingAgent
		tasks[ti] = t
		s.Tasks = tasks
	}

	return s, []runtime.Cmd{
		NewLayInAnswers(LayInAnswersPayload{
			TaskID:   p.TaskID,
			Values:   p.Values,
			FileRefs: p.FileRefs,
		}),
		runtime.Send(agentmsg.NewSpawnAgentRun(agentmsg.SpawnAgentRunPayload{
			TaskID:     p.TaskID,
			Resume:     true,
			SecretRefs: p.SecretRefs,
		})),
	}
}
