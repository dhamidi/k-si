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
`"memory" | "skill" | "reply" | "request" | "transcript"`); identity is `(RunID,
Kind)`. `harvestReconcileSubs` sources one `harvest:<kind>:<run>` per job, emitting
`run-harvest{TaskID, RunID, Kind}`; `handleRunHarvest` DISPATCHES by kind to
`capture-memory`, `store-skill`, `capture-transcript`, a reconstructed
`assemble-reply`, or `mint-ui-request`; each effect ends by emitting
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

## The whole fan-out is now reconciled (built)

- **`capture-transcript`** — reconciled, after `AddArchive` was made idempotent (see
  below). `agent-run-finished` records a `transcript` HarvestJob for EVERY finished
  run (successful, stopped, or failed) instead of firing capture-transcript inline;
  `handleRunHarvest` dispatches the `transcript` kind to capture-transcript, which
  ends with `mark-harvested{transcript}`. This closed a real gap, not just a
  theoretical one: an inline capture lost on a crash between logging
  `agent-run-finished` and the archive write was never re-driven (replay suppresses
  effects), AND `archive-task`'s transcript special-case SKIPS re-archiving it at
  finish — so nothing backstopped it and the transcript was gone. Proven by
  `t/research/transcript-restart-safety.test`, which faults capture-transcript's
  `AddArchive` (`fail content archive`, the write unique to it in the fan-out) and
  recovers it across `crash`/`restart`, with a negative control.

## Also decided in the memory review pass (logged here)

- **Archival duplication (RESOLVED — content-addressed archival, built).** `Files()`
  surfaces `in/memory/*` and provisioned `.claude/skills/*`, so every task used to
  archive a full copy of the whole memory collection (and skills) — `AddArchive` was a
  plain INSERT with no dedup, unbounded growth (tasks × collection). Storage is now
  content-addressed: a `blob(sha256 PRIMARY KEY, bytes)` table holds each unique
  byte-string ONCE, and `archive` is a per-task index referencing it by sha256, with
  `UNIQUE(task_id, filename)` making `AddArchive` idempotent (a re-archived file for
  the same task is a no-op — the property that let capture-transcript be reconciled,
  above). `ArchiveRow` still carries `Bytes` at the API boundary; the blob/index split
  is internal, and readers JOIN blob to reconstitute bytes. Both twins
  (`SQLiteContent`, `MemoryContent`) implement the same semantics. A minimal
  `PRAGMA user_version`-keyed migration runner (the project had none) upgrades an
  existing DB in place — v0→v1 dedups the inline bytes into `blob`, rebuilds `archive`
  without `bytes`, and `VACUUM`s to reclaim the freed pages; a fresh DB is stamped v1
  and skips it. Proven by `t/research/archive-dedup.test` (one blob, two index rows for
  a memory both tasks archive).
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
