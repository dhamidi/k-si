# Decision 018 — the inbox poll cursor lives in the log

## Context

The inbound edge (`cmd/kasi/serve.go`, `pollInbox`) polls Fastmail with a JMAP
high-water mark: `Fetch(sinceState)` returns the mail created since that state
plus the next state to poll from, and the very first `Fetch("")` deliberately
returns the *current* state and no messages, so a fresh deployment does not
ingest the entire historical mailbox.

That cursor was a **local variable**, `state := ""`, reset on every process
start and never persisted. So on a restart the first `Fetch("")` threw away the
last-known state and re-anchored to "now" — and the entire window between
shutdown and restart was never in any `Email/changes` delta. **Mail that arrived
while käsi was offline was silently skipped.** For an always-on, email-driven
assistant that is data loss on every deploy, crash, or reboot.

This also failed an auditability expectation: the position käsi had reached in
its own inbox was ephemeral goroutine state, invisible to the log that is
otherwise the single source of truth (docs/01).

## Decision

**The poll cursor is model state, advanced only through a message.** A new
`email.Model.PollCursor` holds the JMAP Email state, and the only thing that
writes it is the `record-poll-state` message. The poll edge:

1. **seeds** from the replayed model — `state := email.PollCursor(app.View())` —
   instead of `""`, so a restart resumes from the last-processed state and the
   next `Email/changes` returns the offline window; and
2. **advances** by emitting `record-poll-state{state: next}` after a batch is
   routed, so every step of the high-water mark is a log entry.

This is the crash-safety shape the pending outbox (docs/03) and the memory
harvest (decision, `harvest_pending`) already use: a durable value on the model,
reconstructed by full-log replay; the *effect* — the JMAP `Fetch` — runs only
live. The poller goroutine starts after `app.Start` (replay → live), so replay
never runs `Fetch` and merely folds the logged `record-poll-state` entries back
into `PollCursor`. `PollCursor` is absent on pre-018 log entries and decodes as
`""`, so replay of the existing log converges and an initial deployment still
anchors to "now".

The advance is logged **after** the batch is routed and only when the state
actually changed (an idle poll appends nothing). Ordering the cursor advance
last means a crash in the gap replays the *pre-batch* cursor and the poller
re-`Fetch`es mail it already routed — the safe direction, made a no-op by the
idempotency guard below.

## Consequences

- **Offline mail is caught up on restart.** The gap window is fetched by the
  next `Email/changes`, not skipped.
- **The cursor is auditable.** Every position käsi reached in its inbox is a
  `record-poll-state` entry in the log, replayable and inspectable, not a private
  variable.
- **A re-`Fetch` must be idempotent.** Persisting the cursor introduces a narrow
  crash window (batch routed, cursor not yet logged) in which a restart re-routes
  already-processed mail. `route-email` now drops any inbound whose inbox row is
  already laid into a task (`tasks.HasIngestedInbox`), so the re-route creates no
  second task and spawns no second run. InboxIDs are stable across the re-`Fetch`
  because `AddInbox` is idempotent on Message-ID. This closes a fresh instance of
  the self-reply-loop risk (decision-016): a duplicate run is exactly its raw
  material.

## Coverage

- `t/mail/poll-cursor.test` — `record-poll-state` advances `PollCursor`; a
  `crash`/`restart` rebuilds it from the log alone (the poller would resume from
  the saved state, not "now"); it advances monotonically.
- `t/mail/poll-redelivery-idempotent.test` — re-routing an already-ingested inbox
  row creates no second task and does not tick the task's run count.
- The full gate replays every scenario (`--log sqlite`), so the new model field
  is proven replay-convergent.

## Related

- [decision-016](./decision-016-kasi-never-acts-on-its-own-mail.md) — the
  self-reply loop; the re-`Fetch` idempotency guard closes a new instance of it.
- [decision-015](./decision-015-an-interrupted-run-resumes-not-orphans.md) /
  [decision-013](./decision-013-post-finish-effects-are-reconciled-not-fire-and-forget.md)
  — durable state from the log, effect live-only; the pattern this follows.
- `cmd/kasi/serve.go` (`pollInbox`, `route`), `email/message_record_poll_state.go`,
  `email/model_email.go` (`PollCursor`), `tasks/model_task.go`
  (`HasIngestedInbox`).
