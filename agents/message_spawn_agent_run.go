package agents

import (
	"github.com/dhamidi/k-si/agents/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "spawn-agent-run" — sent by tasks; records a new StatusRunning run. It no
// longer launches the harness: the agent-watch source is the SOLE launcher
// (decision-015), driving start-agent-run once it sees a running run with no
// live harness process — a fresh spawn OR a run orphaned by a restart. This
// handler only records the run, carrying the relaunch inputs (Resume,
// SecretRefs) the launcher needs to reconstruct start-agent-run.

func registerSpawnAgentRun(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.SpawnAgentRun, handleSpawnAgentRun)
}

func handleSpawnAgentRun(v runtime.View, s Model, p msg.SpawnAgentRunPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	runID := meta.Offset
	runs := append([]AgentRun(nil), s.Runs...) // copy-on-write
	runs = append(runs, AgentRun{
		ID:         AgentRunID(runID),
		TaskID:     p.TaskID,
		Status:     StatusRunning,
		Session:    sessionFor(p.TaskID),
		Resume:     p.Resume,
		SecretRefs: p.SecretRefs,
	})
	s.Runs = runs
	return s, nil // the agent-watch source is the sole launcher (drives start-agent-run)
}
