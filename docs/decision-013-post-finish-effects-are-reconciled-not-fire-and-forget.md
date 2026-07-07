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

## Generalized across the fan-out (built)

The `HarvestJob` is now KIND-tagged (`{TaskID, RunID, Kind}`, Kind ∈
`"memory" | "skill" | "reply"`); identity is `(RunID, Kind)`. `harvestReconcileSubs`
sources one `harvest:<kind>:<run>` per job, emitting `run-harvest{TaskID, RunID, Kind}`;
`handleRunHarvest` DISPATCHES by kind to `capture-memory`, `store-skill`, or a
reconstructed `assemble-reply`; each effect ends by emitting
`mark-harvested{RunID, Kind}`, which clears only the matching job. `agent-run-finished`
appends a job per kind instead of emitting the effect inline.

- **`store-skill`** — reconciled. Already idempotent (`AddSkill` upsert-by-name +
  `register-skill` upsert). It now ends with `mark-harvested{skill}`. A crash
  mid-store leaves the skill job pending; restart re-drives the whole store.
- **`assemble-reply`** — reconciled, after being made idempotent. `AddOutbox` is now
  idempotent on `message_id` (pre-check + `UNIQUE`, exactly like `AddInbox`), and
  `mark-reply-queued` dedups by outbox id / message id, so a re-driven assemble
  queues the SAME row — never a second reply. The deterministic Message-ID
  (`ReplyMessageID(task, run, domain)`) is the idempotency key. The reply harvest's
  effect runs in the EMAIL module, so it clears its tasks-side job cross-module via
  `tasks/msg.MarkHarvested` (the way `mint-ui-request` emits `register-ui-request`);
  the payload is reconstructed from the logged `Task` in `handleRunHarvest`.

- **`mint-ui-request`** (Flow C) — reconciled. The `crypto/rand` token was a red
  herring: it rides into the log via `register-ui-request` (replay-stable), and the
  reconcile source re-drives only an *incomplete* mint — one where nothing was ever
  registered or sent — so a fresh token on re-drive is correct, not a duplicate.
  `register-ui-request` clears the request job **atomically** with recording the
  `UIRequest` (marker-present ⟺ not-yet-registered), so there is no partial-emit
  window and no dedup is needed. It also stopped driving its reply with an inline
  `assemble-reply` — the last un-reconciled one — and now enqueues a `reply` job;
  `replyCmds` derives the `RequestLink` from the recorded `UIRequest`, so one reply
  reconstruction serves both a normal and a request reply.

Proven by `t/research/skill-restart-safety.test`, `reply-restart-safety.test`, and
`request-restart-safety.test`, each faulting the harvest's scoped read
(`content.AddSkill` for skill — the only op unique to `store-skill`; `Work.Harvest`
for reply and for the mint) and recovering across `crash`/`restart`, with a negative
control.

## Still fire-and-forget (one left, deferred with reason)

- **`capture-transcript`** — `AddArchive` is a plain INSERT (archival dedup is
  itself deferred, below), so re-driving would duplicate the archive row: not cleanly
  idempotent. Left inline and low-harm — the transcript is a re-derivable artifact
  (still in the workspace), not durable user data. Folding it in should wait on the
  content-addressed archival fix.

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
