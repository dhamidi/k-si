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
	// The concurrency cap (decision-016): only the lowest-id MaxConcurrent running
	// runs get a launcher+watcher this diff; the rest stay StatusRunning but launch
	// no process — they queue, and a later diff (when a running run finishes and frees
	// a slot) picks them up. s.Runs is in id-ascending order (spawn-agent-run appends
	// by log offset), so "first N running" is the oldest N. MaxConcurrent 0 is
	// unlimited: active holds every running run, so the gate diffs exactly as before.
	active := activeRuns(s)

	var subs []runtime.Sub
	for _, run := range s.Runs {
		if run.Status != StatusRunning || !active[run.ID] {
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

// activeRuns is the set of running-run ids allowed to hold a live process under
// the concurrency cap (decision-016): the lowest-id MaxConcurrent running runs.
// MaxConcurrent 0 means unlimited — every running run is active — so the sim ring
// and any un-capped deployment behave exactly as before the cap existed.
func activeRuns(s Model) map[AgentRunID]bool {
	active := make(map[AgentRunID]bool)
	n := 0
	for _, run := range s.Runs {
		if run.Status != StatusRunning {
			continue
		}
		if s.MaxConcurrent > 0 && n >= s.MaxConcurrent {
			break
		}
		active[run.ID] = true
		n++
	}
	return active
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
		// Liveness must be answered by the SAME harness that launched (decision-024):
		// resolving by the current default after a restart would ask a Codex run's
		// liveness of Claude and break the relaunch-exactly-once guarantee (decision-015).
		if !e.resolveHarness(run.Harness).IsLive(h) {
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
		harness := e.resolveHarness(run.Harness)
		res := harness.Wait(ctx, h)
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
		// stays exactly per SEAMS, so this is an optional, sim-only method — asserted
		// against the SAME resolved harness that ran the turn (decision-024).
		if m, ok := harness.(interface{ MarkEmitted(Handle) }); ok {
			m.MarkEmitted(h)
		}
	}
}
