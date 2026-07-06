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

	// A run is "stopped" if the model says so — its status was set to stopping by
	// stop-agent-run — not because a mutable harness field claimed it (docs/01:
	// everything the model knows rides on messages). The harness's exit-vs-cancel
	// hint (p.Stopped) corroborates but the model is the authority.
	stopped := p.Stopped

	runs := append([]AgentRun(nil), s.Runs...) // copy-on-write: never mutate the shared snapshot
	if i := s.findRun(AgentRunID(p.RunID)); i >= 0 {
		if runs[i].Status == StatusStopping {
			stopped = true
		}
		if stopped {
			runs[i].Status = StatusStopped
		} else {
			runs[i].Status = StatusFinished
		}
		runs[i].Exit = p.Exit
		runs[i].TranscriptPath = p.TranscriptPath
	}
	s.Runs = runs

	return s, []runtime.Cmd{
		runtime.Send(taskmsg.NewAgentRunFinished(taskmsg.AgentRunFinishedPayload{
			TaskID:         p.TaskID,
			RunID:          p.RunID,
			OutManifest:    p.OutManifest,
			Stopped:        stopped,
			TranscriptPath: p.TranscriptPath,
		})),
	}
}
