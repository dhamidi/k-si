# Decision 012 — the agent store is an edge outside the event log

**Status:** accepted (Flow F design; not yet built)

## Context

An agent needs durable **working data** it mutates in place — a cached SQLite
database, scratch scripts — reused across tasks so it doesn't re-fetch every time
([10](./10-flows.md) Flow F). Skills already give durable *know-how*, but they are
**packed** (a tar blob in the `skill` table, unpacked per run,
[decision-010](./decision-010-skills-content-in-a-table-registry-in-the-model.md)) —
the wrong shape for a live database, which must not be serialised on every run. And
käsi's spine is event sourcing: the model is rebuilt by folding the whole log, with
no snapshots ([01](./01-architecture.md), [03](./03-persistence.md)).

## Decision

Give the agent a **store**: one persistent directory `$STATE/store/`, created once,
never deleted with a task, **symlinked** into every run's workspace as `./store/`.
It is:

- **Never packed** — a real directory of live files (SQLite DBs and their WAL, a
  scratch script), opened and mutated directly. Persistence comes from living
  *outside* the ephemeral per-task workspace, not from serialise/deserialise.
- **Outside the event log, on purpose** — käsi provisions and persists the
  directory but does **not** track or event-source its contents. It is external
  mutable state reached through an **edge**, exactly like the mail edge or Wise
  itself. Replay rebuilds käsi's *model*; the store is the agent's, and was never
  model state, so restart just finds it on disk as the last run left it (Flow F).
- **Global** to start — one shared store for the whole assistant. Per-route
  namespacing (`store/<route>/` + a scoped link) is a later refinement.
- **Shared concurrently** — SQLite WAL handles many readers + one writer; käsi does
  not serialise access.
- **Exposed in the worker prompt** — the agent is told `./store/` is its durable
  memory across tasks, so it caches instead of re-fetching.

## Rationale

A live cache is the agent's memory, not käsi's model — forcing it into the log
would mean serialising a mutating database into an event, which is neither cheap
nor meaningful (the log carries references + light facts, not bulk mutable state,
[03](./03-persistence.md)). Treating it as an edge keeps käsi's replayable core
clean while giving the agent real persistence, the same way the content tables hold
bytes the log only references. Symlink (not copy) is what makes it *live*: writes
land in the one shared directory, and archive-before-delete skips the link so
completing a task can't touch another task's data.

## Consequences

- A persistent-storage edge: `$STATE/store/`, linked into the workspace at spawn
  (the same choke point that provisions skills + secrets), the link skipped by
  `Files`/archival so it is never followed, archived, or deleted.
- The store is **not** covered by replay-convergence or the sim's determinism —
  scenarios that touch it assert on observable effects, not on a rebuilt model.
- Sim twin (the twin rule): a temp-dir or memory store for scenarios.
- Deferred: per-route namespacing, size/retention limits and GC.
