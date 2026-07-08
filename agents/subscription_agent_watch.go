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
		// Two sources per running run. The LAUNCHER is the sole launcher
		// (decision-015): an Await source (like the harvest reconcile) so quiescence
		// covers its launch emit and the start-agent-run provisioning it drives — a
		// non-Await launch would let Settle return before the run is even launched,
		// racing every scenario that reads in/. The WATCHER blocks in Wait for the
		// whole turn, so it MUST stay non-Await or Settle would wait for the turn to
		// finish (deadlocking against the DeliverTurn that finishes it).
		subs = append(subs, runtime.Sub{
			ID:    fmt.Sprintf("agent-launch:%d", run.ID),
			Await: true,
			Run:   launchRun(run),
		})
		subs = append(subs, runtime.Sub{
			ID:  fmt.Sprintf("agent-watch:%d", run.ID),
			Run: watchRun(run),
		})
	}
	return subs
}

// launchRun is the sole launcher (decision-015): if the harness has no live
// process for this run — a fresh spawn, or a run orphaned by a restart
// (start-agent-run is an effect, suppressed on replay) — it drives start-agent-run
// via the launch-agent-run bridge, then returns. IsLive is true only once this
// process has launched the run, so the launch fires exactly once. It is an Await
// source, and it emits then returns immediately, so Settle drains the launch and
// its downstream provisioning before the scenario asserts on the workspace.
func launchRun(run AgentRun) func(ctx context.Context, edges any, emit runtime.Emit) {
	return func(ctx context.Context, edges any, emit runtime.Emit) {
		e, _ := edges.(Edges)
		h := Handle{TaskID: run.TaskID, RunID: int64(run.ID), Session: run.Session}
		if !e.Harness.IsLive(h) {
			emit(NewLaunchAgentRun(LaunchAgentRunPayload{TaskID: h.TaskID, RunID: h.RunID}))
		}
	}
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
