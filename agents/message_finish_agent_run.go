package agents

import (
	taskmsg "github.com/dhamidi/k-si/tasks/msg"

	"github.com/dhamidi/k-si/runtime"
)

// "finish-agent-run" — emitted by agent-watch when the harness exits; records the run, hands off to tasks
const FinishAgentRun = "finish-agent-run"

type FinishAgentRunPayload struct {
	TaskID         int64    `json:"task_id"`
	RunID          int64    `json:"run_id"`
	Exit           int      `json:"exit"`
	TranscriptPath string   `json:"transcript_path"`
	OutManifest    []string `json:"out_manifest"`
	Stopped        bool     `json:"stopped"`
}

func NewFinishAgentRun(p FinishAgentRunPayload) runtime.Msg {
	return runtime.NewMsg(FinishAgentRun, p)
}

func registerFinishAgentRun(mod *runtime.Module) {
	runtime.HandleMsg(mod, FinishAgentRun, handleFinishAgentRun)
}

func handleFinishAgentRun(v runtime.View, s Model, p FinishAgentRunPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	if i := s.findRun(AgentRunID(p.RunID)); i >= 0 {
		if p.Stopped {
			s.Runs[i].Status = StatusStopped
		} else {
			s.Runs[i].Status = StatusFinished
		}
		s.Runs[i].Exit = p.Exit
		s.Runs[i].TranscriptPath = p.TranscriptPath
	}
	return s, []runtime.Cmd{
		runtime.Send(taskmsg.NewAgentRunFinished(taskmsg.AgentRunFinishedPayload{
			TaskID:         p.TaskID,
			RunID:          p.RunID,
			OutManifest:    p.OutManifest,
			Stopped:        p.Stopped,
			TranscriptPath: p.TranscriptPath,
		})),
	}
}
