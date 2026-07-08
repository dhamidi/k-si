package agents

import (
	"github.com/dhamidi/k-si/agents/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "set-max-concurrent-runs" — cap concurrent live agent runs; the rest queue (decision-016)

func registerSetMaxConcurrentRuns(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.SetMaxConcurrentRuns, handleSetMaxConcurrentRuns)
}

func handleSetMaxConcurrentRuns(v runtime.View, s Model, p msg.SetMaxConcurrentRunsPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	s.MaxConcurrent = p.Max
	return s, nil
}
