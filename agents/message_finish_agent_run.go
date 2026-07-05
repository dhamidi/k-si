package agents

import "github.com/dhamidi/k-si/runtime"

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

	return s, nil
}
