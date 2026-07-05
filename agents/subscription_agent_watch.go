package agents

import (
	"context"
	"fmt"

	"github.com/dhamidi/k-si/runtime"
)

// agent-watch — one watcher per running run; emit finish-agent-run when the
// harness exits (or is stopped/crashed).
//
// A pure function from state to the set of sources that should be running, each
// with a stable id; the runtime diffs and starts/stops them (docs/01). A run in
// status "running" declares a watcher; when the run leaves "running"
// (finished/stopped/stopping) the watcher's ctx is cancelled — which is also
// how a stop makes the blocked Wait return.
func agentWatchSubs(v runtime.View, s Model) []runtime.Sub {
	var subs []runtime.Sub
	for _, run := range s.Runs {
		if run.Status != StatusRunning {
			continue
		}
		run := run
		subs = append(subs, runtime.Sub{
			ID:  fmt.Sprintf("agent-watch:%d", run.ID),
			Run: watchRun(run),
		})
	}
	return subs
}

// watchRun builds the watcher body for one running run: block in Harness.Wait
// until the turn completes (or ctx cancels), emit finish-agent-run, then release
// the sim harness's DeliverTurn via MarkEmitted (the quiescence handshake).
func watchRun(run AgentRun) func(ctx context.Context, edges any, emit runtime.Emit) {
	return func(ctx context.Context, edges any, emit runtime.Emit) {
		e, _ := edges.(Edges)
		h := Handle{TaskID: run.TaskID, RunID: int64(run.ID), Session: run.Session}
		res := e.Harness.Wait(ctx, h)
		emit(NewFinishAgentRun(FinishAgentRunPayload{
			TaskID:         h.TaskID,
			RunID:          h.RunID,
			Exit:           res.Exit,
			TranscriptPath: res.TranscriptPath,
			OutManifest:    res.OutManifest,
			Stopped:        res.Stopped,
		}))
		// The sim harness blocks DeliverTurn until finish-agent-run is enqueued
		// (the quiescence handshake, docs/05); release it. The Harness interface
		// stays exactly per SEAMS, so this is an optional, sim-only method.
		if m, ok := e.Harness.(interface{ MarkEmitted(Handle) }); ok {
			m.MarkEmitted(h)
		}
	}
}
