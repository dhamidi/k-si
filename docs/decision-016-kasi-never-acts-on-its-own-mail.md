# Decision 016 — käsi never acts on its own mail, and runaway work is bounded

## Context — a SEV1 self-reply loop

A mail that CC'd käsi's own address (`kasi@decode.ee`) made käsi a **participant** on
the resulting task. käsi then replied to *all* participants — itself included — the
inbound poller fetched that self-addressed copy, threaded it back onto the same task,
`append-to-task` spawned another run, which replied again. An infinite loop: in one
incident it sent **94 real emails in ~7 minutes** and launched **392 agent
subprocesses**, OOM-killing the box (7.7 GB / 2 vCPU) repeatedly until the service was
stopped.

The loop needed **four** failures lined up; breaking **any one** stops it:

1. **käsi replies to itself** — the reply recipient set is `Task.Participants`, which
   included käsi's own address whenever it was CC'd. Self was never stripped.
2. **The poll re-ingests käsi's own outbound** — the self-addressed copy lands in
   käsi's mailbox and is routed back in as if it were fresh inbound.
3. **`append-to-task` is ungated** — the threaded-append path spawned a run for any
   sender, käsi's own address included.
4. **No breaker** — runs spawn faster than they finish, so a loop escalates into
   hundreds of concurrent processes → OOM.

## Decision — defense in depth, each guard independent

The self identity is already model state: `tasks.Model.ReplyFrom` (set by
`set-reply-from` / serve `-from`). Every guard is a **pure function of the model** —
no edge or mutable state — and addresses compare through `mime.SameAddress`, which
matches the bare mailbox case-insensitively and tolerates display-name forms. An
**empty** self (the sim ring, which leaves `ReplyFrom` unset) matches nothing, so
offline tests are unaffected.

**Fix A — self is never a participant, so never a recipient.** `create-task` and
`append-to-task` build `Participants` through `dropSelf(…, ReplyFrom)`, so käsi's own
address never enters the set. Every reply path (`agent-run-finished`,
`run-harvest`→`replyCmds`) derives its recipients from `Participants`, so excluding
self once excludes it everywhere. Belt-and-braces: `assemble-reply` also filters
`From` out of `To`, so no code path can emit a reply addressed to käsi.

**Fix B — inbound from käsi's own identity drives nothing.** `route-email` drops a
mail whose sender is `ReplyFrom` at the earliest choke point, before create OR append.
This closes the poll-re-ingestion hole even if a self-copy slips through — a
self-message must never drive a run.

**Fix C — two runaway breakers, off by default, armed by serve.**

- *Per-task loop breaker.* `Task.Turns` counts the runs a task has spawned;
  `append-to-task` pauses the task (new terminal status **`Paused`**, surfaced at the
  top of the browse list) instead of spawning once `Turns` would exceed
  `Model.LoopGuard`. This catches a loop Fix A/B cannot see: a benign back-and-forth
  with an **allowlisted auto-responder**, where the sender is a real human address.
  The check trips at spawn time, so the extra process never starts. `set-loop-guard`
  (serve `-max-task-runs`, default 20).
- *Global concurrency cap.* `agents.Model.MaxConcurrent` caps how many runs hold a
  live process at once. The sole launcher (decision-015) starts only the lowest-id
  `MaxConcurrent` running runs; the rest stay `StatusRunning` but hold no process
  (queued) until a slot frees, then a later subscription diff picks them up. This
  composes with restart-resume: a restart that rebuilds 50 orphaned runs launches them
  `MaxConcurrent` at a time rather than forking a bomb. `set-max-concurrent-runs`
  (serve `-max-concurrent-runs`, default 2 — this box realistically runs 1–2 claude
  processes).

`0` disables either breaker; both default to `0` in the model, so the sim ring and the
gate launch and spawn exactly as before. Production arms them via the serve flags.

## Why all four, not just the load-bearing two

Fix A and Fix B each independently kill the *known* self-CC loop, and both are tiny and
pure. Fix C is the backstop for loops those two can't see — the auto-responder
ping-pong (a real allowlisted human on the other end) and any future spawn path — and
it bounds the blast radius (and the OOM) of *any* loop regardless of cause. A SEV1 that
took the box down repeatedly earns the redundancy.

## Consequences

- A single email thread is capped at `LoopGuard` agent turns; a genuinely long thread
  that hits the cap pauses and the human starts a fresh mail. A paused task is terminal
  until an operator intervenes (it still archives via the completion link).
- Self-addressed test fixtures are now invalid by construction. The two ring-3 probes
  and the ring-2 mail-roundtrip switched their initiator to `kasi+probe@decode.ee` — a
  plus-addressed alias that is **not** käsi's identity to `SameAddress`, yet still
  käsi's own mailbox, so a live run's reply stays self-contained (no external
  recipient). mail-roundtrip was re-recorded live to match.

## Coverage

- `t/mail/self-cc-no-loop.test` — a mail CC'ing käsi's own address: the reply is
  addressed to the human only (Fix A), and käsi's own re-ingested copy drives no run,
  no second reply, no new task (Fix B).
- `t/research/loop-breaker.test` — with `LoopGuard 2`, a task pauses on the run that
  would exceed it; a paused task ignores further threaded mail.
- `t/research/concurrency-cap.test` — with `MaxConcurrent 1`, a second task's harness
  is not started (no env) until the first finishes and frees the slot (`run-env`
  distinguishes launched from queued).

## Related

- [decision-015](./decision-015-an-interrupted-run-resumes-not-orphans.md) — the sole
  launcher this cap gates; the two compose (a restart launches orphans `MaxConcurrent`
  at a time).
- `mime.SameAddress`, `tasks.ReplyFrom`, `email/message_route_email.go` (Fix B),
  `tasks/message_create_task.go` + `message_append_to_task.go` (Fix A + loop breaker),
  `agents/subscription_agent_watch.go` (concurrency cap).
