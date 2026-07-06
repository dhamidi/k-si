package msg

import "github.com/dhamidi/k-si/runtime"

// "agent-run-finished" — sent by agents when a run exits; harvest out/, capture the transcript, reply
const AgentRunFinished = "agent-run-finished"

type AgentRunFinishedPayload struct {
	TaskID         int64    `json:"task_id"`
	RunID          int64    `json:"run_id"`
	Exit           int      `json:"exit"`
	OutManifest    []string `json:"out_manifest"`
	Stopped        bool     `json:"stopped"`
	TranscriptPath string   `json:"transcript_path"`
}

func NewAgentRunFinished(p AgentRunFinishedPayload) runtime.Msg {
	return runtime.NewMsg(AgentRunFinished, p)
}
