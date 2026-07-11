# Decision 024 — harness selection, and a generalized session identity

## Context

käsi does not implement an agent loop; it shells out to an **official harness**
([05](./05-agents-and-tasks.md)). Until now there was exactly one: `agents.Edges`
carried a single `Harness` field, one real twin (`harness_claude.go`) shelled out
to `claude`, and the Sim/Recorded/Recording twins were harness-agnostic decorators
over that one interface. Every launch/watch/signal/start site resolved the session
deterministically from `sessionFor(taskID)` and **discarded** the `Handle.Session`
that `Start` already returned.

We want an operator to run **OpenAI's Codex CLI** — authenticated with their
ChatGPT subscription — as the worker harness, as a *selectable alternative* to
Claude, without käsi ever implementing an agent loop and without disturbing any of
the harness-edge invariants: secrets only at the edge
([decision-004](./decision-004-secrets-are-written-at-the-web-edge-resolved-at-the-agent-edge.md)),
relaunch-exactly-once ([decision-015](./decision-015-an-interrupted-run-resumes-not-orphans.md)),
per-turn `out/` framing ([decision-019](./decision-019-out-is-a-per-turn-outbox.md)),
and settings as logged, guarded-seeded contributions
([decision-020](./decision-020-settings-are-typed-contributions-rendered-by-a-runtime-form-engine.md)).

The single-field harness and the discarded session are the whole gap.

## Decision

### 1. A harness registry, pinned per task

`Edges.Harness Harness` becomes `Edges.Harnesses map[string]Harness`, built at
boot in `cmd/kasi`. `AgentRun` gains a `Harness string` field, **pinned once per
task**: the spawn handler (which has the View) stamps it from the `worker_harness`
setting for a fresh run, and **inherits the task's prior run's pin on a resume** —
so one task ⇔ one session ⇔ one harness, matching `sessionFor`'s
one-session-per-task invariant. A `worker_harness` setting (default `claude`, a
`KindChoice` over `agents.HarnessNames()`) is the operator's control, guarded-seeded
from a `-harness` flag exactly like `-from`/`-base-url`: seeded only when
`WorkerHarnessOf(view) == ""`, the empty string being the clean unset sentinel.

Empty resolves to the built-in `claude`, and `AgentRun.Harness` /
`Model.WorkerHarness` are both `omitempty`, so a deployment that never chose keeps
its log **byte-identical** and its cassettes untouched.

### 2. Effects never read the model — the name rides the payload

`startAgentRunEffect`/`signalAgentRunEffect` have no View, so they cannot read the
setting or the run's pin. The name is resolved by the *handlers that have the View*
— `handleSpawnAgentRun` pins it, `handleLaunchAgentRun` copies `run.Harness` into
`StartAgentRunPayload`, the stop handler copies it into `SignalAgentRunPayload` —
and the effect selects `Harnesses[name]` through `resolveHarness`, which defaults an
empty or pinned-but-missing name to `claude` rather than a nil harness.

### 3. `IsLive` resolves the same instance that launched

All five edge calls — `Start`, `Resume`, `Wait`, `IsLive`, `Signal` (and the
sim-only `MarkEmitted` type-assert) — dispatch through the map by the run's pinned
name. Resolving by the *current default* after a restart would answer a Codex run's
liveness against Claude and break the relaunch-exactly-once guarantee
(decision-015).

### 4. Session identity, generalized and cassette-safe

`Start` already returns `Handle.Session`. We stop discarding it. `Resume` is handed
the run's `Session` (via `StartAgentRunPayload.Session`) instead of a hardcoded
`sessionFor(p.TaskID)`, and the effect **captures** the `Handle` that `Start`/
`Resume` returns. It persists the session through a new **logged** `record-session`
message (mirroring `record-notify-token`), because the effect is suppressed on
replay and only a logged message survives it.

Crucially the emit is **conditional — only when the returned session differs from
`sessionFor(taskID)`**. Claude, the sim, and the recorded twin all return
`sessionFor`, so they never emit it: their logs stay byte-identical, committed
cassettes are untouched, and twin parity comes for free. Only a Codex run — which
mints its own session id — logs a `record-session`, and the next turn's Resume
reads the correct `run.Session` from the model.

### 5. Codex is one new real twin

`harness_codex.go` clones `harness_claude.go`: its own binary and one-shot exec
flags, its own session/resume semantics (returning its minted session from `Start`),
its own new-turn framing equivalent to `resumePreamble` (so a resumed Codex turn
writes a fresh `reply.txt` and does not re-send the prior reply — decision-019),
`SysProcAttr{Setpgid:true}` + `kill(-pgid, …)` graceful-then-hard, and a standing
prompt that avoids the Claude-isms the shared prompt leaks. `ResetOut` stays
outside the adapter (harness-agnostic). Sim/Recorded/Recording need no per-harness
code; they are registered under *every* selectable name so a scenario pinning
`codex` runs the twin — the harness conformance suite (`t/research/harness-codex.test`)
runs the whole lifecycle over the pin unchanged.

## Consequences

- **Secrets stay at the edge (decision-004).** Codex's ChatGPT-subscription auth
  rides the CLI's own logged-in credential in the process environment — never a
  token in the model, the log, a message, or an `out/` file — exactly as `NewClaude`
  assumes `claude` is authenticated.
- **Replay convergence holds.** The new scalar fields marshal stably and omit when
  empty; `Runs` stays an ordered slice; the conditional `record-session` keeps the
  Claude/sim/recorded logs identical, which the replay-convergence and log-cassette
  standing checks verify.
- **The twin rule holds.** The same scenarios run over any adapter; only one real
  twin was added.
- **What this decision does not cover.** Codex-native skills discovery, a
  harness-dispatched transcript reader, a per-run `CODEX_HOME` for the subscription
  blob, and rollout-scan session recovery are follow-on work, each riding this
  registry rather than bending it.

This is Elm-in-Go effects-over-edges to the letter: the harness *name* and
*session* are transient decision state carried on messages/payloads by
handlers-with-a-View, materialized at the single start-agent-run choke point, and
persisted back into the log only through messages.
