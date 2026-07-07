package agents

import (
	"github.com/dhamidi/k-si/agents/msg"
	"github.com/dhamidi/k-si/memory"
	"github.com/dhamidi/k-si/runtime"
)

// "spawn-agent-run" — sent by tasks; start (or resume) the worker harness for a task turn

func registerSpawnAgentRun(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.SpawnAgentRun, handleSpawnAgentRun)
}

func handleSpawnAgentRun(v runtime.View, s Model, p msg.SpawnAgentRunPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	runID := meta.Offset
	s.Runs = append(s.Runs, AgentRun{
		ID:      AgentRunID(runID),
		TaskID:  p.TaskID,
		Status:  StatusRunning,
		Session: sessionFor(p.TaskID),
	})
	return s, []runtime.Cmd{
		NewStartAgentRun(StartAgentRunPayload{
			TaskID:     p.TaskID,
			RunID:      runID,
			Resume:     p.Resume,
			SecretRefs: p.SecretRefs,
			// Read the whole memory collection from the model HERE (the handler has
			// the View) and carry it through the Cmd to the workspace edge, which
			// provisions it (feature-memory.md). The read is pure; the write is at the edge.
			Memory: memory.All(v),
		}),
	}
}
