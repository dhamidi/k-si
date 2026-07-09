package agents

import (
	"github.com/dhamidi/k-si/apps"
	"github.com/dhamidi/k-si/memory"
	"github.com/dhamidi/k-si/runtime"
)

// "launch-agent-run" — the bridge from the agent-watch source (which can only
// emit a Msg, not a Cmd) to the start-agent-run effect. The source drives this
// when the harness has no live process for a StatusRunning run — a fresh spawn
// or a run orphaned by a restart (decision-015). Its handler, which has the
// View, reconstructs start-agent-run from the run's recorded relaunch inputs.
const LaunchAgentRun = "launch-agent-run"

type LaunchAgentRunPayload struct {
	TaskID int64 `json:"task_id"`
	RunID  int64 `json:"run_id"`
}

func NewLaunchAgentRun(p LaunchAgentRunPayload) runtime.Msg {
	return runtime.NewMsg(LaunchAgentRun, p)
}

func registerLaunchAgentRun(mod *runtime.Module) {
	runtime.HandleMsg(mod, LaunchAgentRun, handleLaunchAgentRun)
}

// handleLaunchAgentRun reconstructs start-agent-run for the run named in the
// payload, reading the run's recorded Resume + SecretRefs and re-reading the
// whole memory collection at launch time (the same pure read spawn-agent-run
// used to do). It does NOT mutate the model — start-agent-run mints/records the
// fresh notify token in its effect, so a restart-resume simply gets a new token
// (the dead process's is gone). If the run vanished, it is a no-op.
func handleLaunchAgentRun(v runtime.View, s Model, p LaunchAgentRunPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	run, ok := Run(v, AgentRunID(p.RunID))
	if !ok {
		return s, nil
	}
	return s, []runtime.Cmd{
		NewStartAgentRun(StartAgentRunPayload{
			TaskID:     p.TaskID,
			RunID:      p.RunID,
			Resume:     run.Resume,
			SecretRefs: run.SecretRefs,
			Memory:     memory.All(v),
			Apps:       apps.Running(v),
		}),
	}
}
