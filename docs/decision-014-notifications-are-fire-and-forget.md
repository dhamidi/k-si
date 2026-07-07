# Decision 014 — a notification is fire-and-forget, the deliberate inverse of 013

## Context

[Decision 013](./decision-013-post-finish-effects-are-reconciled-not-fire-and-forget.md)
established the rule for durable post-finish work: it must be reconcilable from
logged model state, never fire-and-forget, so a restart can't silently drop it.
The reply, the skill store, the memory harvest, and the transcript all obey it.

A **notification** (docs/feature-notifications.md) breaks the other way, and on
purpose. An agent mid-task runs `kasi notify "Smart-ID code 4271 — approve within
60s"`; the message must reach the owner *now* and needs nothing back. The canonical
case is a two-factor prompt with a countdown: the value is worthless the moment the
timer expires.

If a notification took decision-013's reconciled path — write a pending outbox row,
let a subscription send it, retry until a completion clears it — then a crash between
logging the send and transmitting it would cause the notification to be **re-sent on
restart**. For a durable reply that is exactly right. For a 2FA code it is wrong: the
restart happens seconds-to-minutes later, the code has expired, and re-sending it
delivers a stale, confusing message. The run that asked for it died in the same crash,
so there is nothing left that the late notification serves.

## Decision

A notification sends **fire-and-forget**, not through the outbox/reconcile path.

- The control endpoint injects a logged `notify-user` message (so replay and audit
  show *that* the owner was told, when, and by which run — decision-013's audit
  benefit is kept).
- `notify-user` (tasks) derives the threaded mail from the `Task` and returns one
  `send-notification` (email) command.
- `send-notification` builds the MIME and calls `Mail.Submit` **directly**, emitting
  nothing. On replay the effect is suppressed, so it **never re-sends** — the very
  property that would be a bug for a reply is the correct behavior here.

This is allowed by decision-013's own escape clause: *"Fire-and-forget is acceptable
only when the loss is tolerable."* A dropped notification is tolerable — better than a
stale re-send — because the message is time-bounded and one-way. If a notification
ever needs to be durable, it isn't a notification; it's a reply or a request, and
those already have the reconciled path.

## What a notification is NOT

- **Not a reply.** It carries no completion link, does not mark a reply queued, and
  does not change task status. The task keeps running.
- **Not a request.** Nothing routes back. The owner's action (e.g. tapping approve on
  their phone) reaches the *browser* the agent is driving, not käsi. The agent waits
  on the page advancing, not on käsi — so käsi's no-blocking rule holds
  (docs/feature-notifications.md).
- **Not recorded in the thread References.** The notification threads *into* the
  conversation (In-Reply-To the last message, References the chain) so it shows up in
  context, but its own Message-ID isn't appended to `Task.References`: it's one-way,
  nobody replies to it, and a reply that did arrive still carries the original chain
  and routes by that.

## The per-run capability

`kasi notify` never holds the Fastmail credential, the owner's address, or the task
id-of-record; it reads three env vars set at run start and POSTs the message:

- `KASI_TASK_ID` — the task the run belongs to.
- `KASI_CONTROL_URL` — the server's loopback control origin (derived from `serve
  -addr`; `0.0.0.0`/`::` collapse to `127.0.0.1`). Host-gated: only something on the
  box can reach it.
- `KASI_NOTIFY_TOKEN` — a 128-bit crypto/rand token minted **per run** at the
  `start-agent-run` edge, injected into the agent env, and recorded onto the
  `AgentRun` model via `record-notify-token` (so it is replay-stable and readable
  live). The token rides into the log as a value, exactly like the completion and
  UI-request tokens.

`POST /control/notify` accepts a notification only when the token matches — constant-
time — the token of the currently-**running** `AgentRun` for `KASI_TASK_ID`. So an
agent can only notify as its own task (it has only its own run's token), and not after
its run has ended (the run is no longer running). The endpoint lives on the existing
host-gated `web.Server`, which already holds the live `App` and injects messages the
same way the web UI and the mail poller do (`app.Send`).

## Why the endpoint, not direct mail

`kasi notify` routes through the server rather than sending mail itself so that: the
Fastmail credential never enters the agent's environment; the one mail path is reused
instead of duplicated; and the notification lands in the log for replay and audit.

## Related

- [decision-013](./decision-013-post-finish-effects-are-reconciled-not-fire-and-forget.md)
  — the rule this deliberately inverts, under its own escape clause.
- [feature-notifications.md](./feature-notifications.md) — the user-facing design.
- `email/command_send_notification.go`, `tasks/message_notify_user.go`,
  `agents/command_start_agent_run.go` (token mint + env), `web/server.go`
  (`/control/notify`), `cmd/kasi/notify.go` (the client).
