# Decision 013 — post-finish effects are reconciled, not fire-and-forget

## Context

`agent-run-finished` (a logged message) fans out into effects that read the run's
workspace and produce durable state: `capture-transcript`, `store-skill`,
`assemble-reply`, and (Flow: memory) `capture-memory`. In käsi's runtime an effect
runs **only live** — replay re-derives the command from the folded model but
**suppresses the effect** (docs/01). The effect's results re-enter the log only as
the messages it `emit`s while running.

That opens a crash window. If the process dies between `agent-run-finished` being
appended and one of these effects finishing, on restart replay re-derives the
command, suppresses it, and the messages the effect would have emitted were never
logged — so the work is **lost forever**. For the memory harvest this is
especially bad: the harvest's `remember`/`forget` are the *only* durable record of
what the agent learned or dropped, and nothing user-facing signals the loss.

**Application restarts must always be safe** — a restart may never silently drop
durable work.

## Decision

Durable post-finish work must be **reconcilable from logged model state**, using
the same shape email delivery already uses (`email/subscription_outbox_reconcile.go`):

1. The triggering handler records a **pending marker** in the model (logged state),
   instead of firing the effect directly.
2. A **reconcile subscription** — a pure function from the model to the set of
   sources that should run — emits, once per pending marker, a message that drives
   the effect. Because a subscription's `emit` takes a `Msg`, not a `Cmd`, a tiny
   **bridge message** turns it into the command (the `send-outbox`→`send-email`
   pattern).
3. The effect **clears the marker only at its end**, via a logged completion
   message.
4. The effect's emissions are **idempotent**, so re-running the whole thing is safe.

A crash anywhere before the completion message leaves the marker pending; restart's
replay rebuilds it and the source fires again. This is the guarantee the pending
outbox row gives an unsent reply.

**Applied to the memory harvest (built, decision-013):** `agent-run-finished`
appends a `HarvestJob{TaskID, RunID}` to `tasks.Model.HarvestPending` instead of
emitting `capture-memory`; `harvestReconcileSubs` emits `run-harvest` per pending
job; its handler returns the `capture-memory` command; the effect ends with
`mark-harvested`, which removes the job. `remember` (upsert by name) and `forget`
(no-op-if-absent) are idempotent. Proven by `t/research/memory-restart-safety.test`,
which faults the harvest's workspace read to exercise the true crash window and
recovers it across `crash`/`restart` (with a negative control).

## The general rule

A post-finish effect that produces durable state and must survive a restart needs:
(a) idempotent emissions, (b) a logged pending marker, and (c) a reconcile
subscription that re-drives it until a logged completion clears it. Fire-and-forget
is acceptable only when the loss is tolerable, or the work is independently
reconciled downstream.

## Scope and follow-ups

The same crash window still exists, pre-existing, for the other `agent-run-finished`
effects (flagged in `tasks/message_agent_run_finished.go`):

- **`store-skill`** — idempotent already (`AddSkill` upsert-by-name +
  `register-skill` upsert). Extend the same reconciliation: a skill-harvest pending
  marker driven to completion. Low risk; recommended next.
- **`assemble-reply`** — **not** idempotent: re-driving it would queue a *second*
  reply. The *send* is already reconciled (the outbox), but the pre-assemble window
  is exposed. It needs run-keyed idempotency (assemble at most once per run — e.g.
  keyed on the run id / a pre-minted Message-ID) before it can be reconciled.
- **`capture-transcript`** — rewrites the same transcript; effectively idempotent,
  lowest priority.

Until then, a crash in that narrow window can still drop a skill or a reply. Making
the whole fan-out reconcilable is the way to hold "restarts always safe" across all
of it, not just memory.

## Also decided in the memory review pass (logged here)

- **Archival duplication (deferred).** `Files()` surfaces `in/memory/*` and
  provisioned `.claude/skills/*`, so every task archives a full copy of the whole
  memory collection (and skills) — `AddArchive` is a plain INSERT and `archive.sha256`
  is not UNIQUE. Unbounded growth (tasks × collection). The fix is systemic
  content-addressed archival (dedup by content hash), covering attachments, skills,
  and memory alike — its own task, not memory-specific.
- **Collection size cap (deferred).** No bound on the number/size of memories, all
  carried into every run and logged. A per-note and per-collection guard is a
  scaling refinement, not a correctness bug.
- **Owner can wipe a description (accepted).** Editing a memory in `/memory` and
  dropping its frontmatter clears the description. The owner owns the raw file; this
  is by design, not guarded.

## Related

- `email/subscription_outbox_reconcile.go` — the precedent this generalizes.
- [decision-012](./decision-012-the-agent-store-is-an-edge-outside-the-log.md),
  [feature-memory.md](./feature-memory.md).
