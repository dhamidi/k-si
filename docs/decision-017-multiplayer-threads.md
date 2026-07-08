# Decision 017 — a task is a multi-party thread; everyone but käsi is a participant

## Context

A task's `Participants` set drives who käsi reply-alls to and who may drive the thread
(a threaded reply from a participant appends and spawns a run; docs/04). Until now the
set was built from **sender + Cc only** — the poller extracted the `From` and `Cc`
headers but never the `To`. So when someone emailed a human and merely CC'd käsi, or
emailed several people with käsi among them, the plain **To** recipients were dropped:
käsi replied only to the sender-plus-Cc, and a To-recipient's later reply was ignored
as a non-participant. Group threads silently lost their members.

[Decision 016](./decision-016-kasi-never-acts-on-its-own-mail.md) had just made käsi's
own identity a hard exclusion from the participant set (the SEV1 self-reply fix). That
is the invariant multiplayer must preserve: **käsi joins the conversation, but never
itself.**

## Decision

Participants are **everyone on the envelope — From + To + Cc — minus käsi's own
addresses.** The poller (and the sim `deliver` / `deliver { raw }` twins) now carry the
`To` recipients on `route-email`, and `create-task` / `append-to-task` fold `To` into
the set beside `Cc` before excluding käsi.

"käsi's own addresses" (`dropOwn`) are two things:

1. **`ReplyFrom`** — käsi's deliverable identity (`set-reply-from` / serve `-from`), the
   loop-critical self. This is the only exclusion that matters in production, where käsi
   has a single real address and every inbound necessarily carries it in To or Cc.
2. **Any address on the internal route domain (`kasi.test`)** — the `pay@`, `research@`
   addresses users send *to* in order to select a route. They are käsi's own receiving
   addresses, never human participants, and this is what keeps every existing
   `to <route>@kasi.test` scenario's participant set unchanged. No `.test` address ever
   reaches a live deployment, so this clause is a no-op in production.

Routing is untouched: `Recipient` stays the address the mail was delivered to käsi at
(which selects the route), distinct from the participant set. `To` is absent on
pre-decision-017 log entries and decodes as nil, so replay of the existing log yields
the old From+Cc participants — convergence holds.

## Consequences

- käsi reply-alls a group thread: a mail To alice CC käsi makes both the sender and
  alice participants, and käsi replies to both.
- A participant who is **not** on the initiator allowlist can drive a threaded task
  (their reply appends and spawns a run). This is not new — a Cc'd non-allowlisted
  participant could already do this (see `t/pay/invoice-confirmation.test`, where Alice
  joins by being CC'd) — decision-017 only extends it from Cc to To. The allowlist is
  the spam boundary on *starting* a task; once an allowlisted sender opens a thread, the
  people they included are its participants. Runaway participation (e.g. an
  auto-responder) is bounded by the decision-016 loop breaker and concurrency cap.
- The self-exclusion now depends entirely on käsi knowing its own identity
  (`ReplyFrom`). Production always sets `-from` (serve requires it for `-send`); a
  scenario that delivers to käsi's real address must `set-reply-from` to match, or that
  address would (correctly, by the rule) look like a stranger and become a participant.
  `t/mail/inbound-real.test` was updated to set it.

## Coverage

- `t/mail/multiplayer.test` — a To recipient (not sender, not Cc, not allowlisted)
  becomes a participant and is reply-all'd; that participant then drives the thread with
  a reply; käsi's own address never enters the set.
- `t/pay/invoice-confirmation.test` (unchanged) — the pre-existing Cc-participant case
  still holds.

## Related

- [decision-016](./decision-016-kasi-never-acts-on-its-own-mail.md) — the self-exclusion
  invariant this preserves while widening the set; `dropOwn` is the shared exclusion.
- `email/message_route_email.go`, `tasks/message_create_task.go` +
  `message_append_to_task.go` (participant assembly), `cmd/kasi/serve.go` (the poller
  extracting `To`).
