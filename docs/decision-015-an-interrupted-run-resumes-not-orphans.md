# Decision 015 — an interrupted agent run resumes on restart, it is not orphaned

## Context

[Decision 013](./decision-013-post-finish-effects-are-reconciled-not-fire-and-forget.md)
made käsi's *post-finish* effects (the harvest fan-out) survive a restart: each is
recorded as a pending job on the model, and a reconcile subscription re-drives it
until a completion clears it. But it never covered the run **itself** — the harness
launch.

`start-agent-run` (the effect that launches *and registers* the worker process) runs
**only live**; replay suppresses it (docs/01). So a run interrupted before it
finished — the process killed mid-turn by a crash or a redeploy — was orphaned:
replay rebuilt the `AgentRun` as `StatusRunning`, the `agent-watch` subscription
re-declared its watcher, but the watcher's `Harness.Wait` blocked forever on a
`Start` that replay had suppressed and nothing re-drove. The task sat in
`awaiting-agent` permanently. This was hit for real by a redeploy that stopped the
service while a run was mid-turn.

**It was invisible to the gate for two reasons.** Every crash scenario crashes
*after* the run finished (`awaiting-user`), testing the harvest — none crashed
*mid-run*. And the two harness twins disagreed: the **sim** harness auto-registered
a missing run inside `Wait` (silently modelling a resume), while the **real** Claude
harness blocked. A twin-parity hole that no ring-1/2 test could see, because ring 1/2
run the sim.

**Application restarts must always be safe** — a restart may never strand
in-flight work, the run included.

## Decision

The harness launch becomes reconcilable from `StatusRunning`, exactly as the harvest
is reconcilable from a pending job. There is **one launcher**: the `agent-watch`
source.

1. `spawn-agent-run` no longer launches inline. It only records the `AgentRun`
   (`StatusRunning`), now carrying the two inputs a relaunch needs beyond what it
   already had (`Session`, `NotifyToken`): **`Resume`** and **`SecretRefs`**, both of
   which already rode in the logged `spawn-agent-run` payload, so both survive replay.
2. The `agent-watch` source is the **sole launcher**. Its body, before `Wait`, asks
   the harness `IsLive(h)` — is there a live process for this run *in this process*?
   If not (a fresh spawn, or a run orphaned by a restart), it emits `launch-agent-run`,
   whose handler reconstructs `start-agent-run` from the `AgentRun` (`Resume`,
   `SecretRefs`) plus the memory collection read from the model and the control URL
   from the edge, and returns it as the command. Then it `Wait`s; `awaitRun` blocks
   until `start-agent-run` registers the run, as it always has.
3. `IsLive` is true only once this process has launched the run, so the source fires
   the launch **exactly once per run per process** — no double-launch, and no
   double-mint of the per-run notify token.

Because both twins now launch through the identical path, the sim harness's
auto-register-in-`Wait` special case is **removed**: its `Wait` blocks until the
source-driven `start-agent-run` registers the run, like the real harness. The
divergence that hid the bug is gone.

On restart, replay rebuilds every in-flight run as `StatusRunning`; at go-live the
sources are declared, see `IsLive` false (a fresh process has an empty registry), and
re-drive `start-agent-run(resume)`. The session resumes and the turn finishes as if
the restart never happened. The worker workspace (in/, out/, provisioned skills and
memory) survives on disk, so a resume re-attaches to it; a fresh notify token is
minted for the resumed process, which is correct — the dead process's token is gone.

## Why resume, not fail-and-restart

The interrupted run produced nothing durable (no `agent-run-finished`, so no harvest,
reply, or skill), so resuming re-does no committed work. Failing the run and asking
the human to resend would lose the turn's in-session progress and surface a restart —
an implementation detail — as a user-visible failure. A restart must be invisible.

## The general rule (extends 013)

Durable work that must survive a restart has to be **reconcilable from logged model
state** — and that now includes the agent run's own launch, not just its post-finish
effects. Any effect that is fire-once and load-bearing needs a model condition that
re-derives it on replay; for the launch, that condition is `StatusRunning` + the
edge's `IsLive` answer.

## Coverage

- `t/research/run-restart-safety.test` — crashes a run **mid-turn** (before the turn
  is delivered, `awaiting-agent`), restarts, and asserts the run resumes and finishes
  into exactly one reply, with a no-crash negative control. With the sim's
  auto-register removed, this passes **only** because the sole launcher relaunches the
  run — so it is a true regression guard for both the fix and the twin parity.
- A ring-3 probe covers the **real** Claude harness resume, the surface the gate
  cannot reach (the same reason the memory-prompt contract needed a probe).

## Related

- [decision-013](./decision-013-post-finish-effects-are-reconciled-not-fire-and-forget.md)
  — the post-finish reconciliation this extends to the run level.
- `agents/subscription_agent_watch.go` (the sole launcher), `agents/harness_sim.go`
  and `agents/harness_claude.go` (`IsLive`, the twin parity),
  `agents/message_launch_agent_run.go`, `agents/message_spawn_agent_run.go`.
