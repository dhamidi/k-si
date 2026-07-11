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
		Harness:    pinHarness(v, s, p),
	})
	s.Runs = runs
	return s, nil // the agent-watch source is the sole launcher (drives start-agent-run)
}

// pinHarness decides the harness a run is pinned to for its whole life
// (decision-024): one task ⇔ one session ⇔ one harness. A fresh run (Resume false)
// takes the operator's current worker_harness choice; a resume INHERITS the pin of
// the task's most recent prior run, so a task can never switch harnesses mid-flight
// even if the setting changed between turns. Empty is the unset sentinel — it
// resolves to the built-in harness and, being omitted from the log, keeps default
// deployments' runs byte-identical to before the registry existed.
func pinHarness(v runtime.View, s Model, p msg.SpawnAgentRunPayload) string {
	if p.Resume {
		for i := len(s.Runs) - 1; i >= 0; i-- {
			if s.Runs[i].TaskID == p.TaskID {
				return s.Runs[i].Harness
			}
		}
	}
	return WorkerHarnessOf(v)
}
