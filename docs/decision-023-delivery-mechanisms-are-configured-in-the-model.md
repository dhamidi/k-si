# Decision 023 — delivery mechanisms are configured in the model, edges stay static

## Context

Chapter [04](./04-email.md) describes an email edge that is frozen at boot. There
is exactly one outbound `Mail` (a spool in development, Fastmail JMAP with `-send`)
and one inbound source (the Fastmail poller, gated by `-poll`). *Which* provider,
and whether it is live, are launch flags decided before the log is even open.

We want to add **ForwardEmail** as a *separate* provider — coexisting with
Fastmail, each enabled on its own — and to **set it up through käsi's web UI**
without a redeploy: enter the credential, flip it on. More generally, we want the
email edge to be pluggable (Fastmail, ForwardEmail, spool today; exe.dev Maildir,
raw SMTP later).

Two forces collide. Configuration that is set up through käsi belongs in the model
and must take effect at runtime — that is the whole point of the settings module
([decision-020](./decision-020-settings-are-typed-contributions-rendered-by-a-runtime-form-engine.md)).
But edges are constructed once, in `main.go`, and the reducer never starts a
goroutine or swaps an edge ([01](./01-architecture.md)); decision-020 kept
`-poll`/`-send` as flags *precisely because they activate edges*. This decision
resolves that tension for a configured provider.

An inbound **webhook** (real-time push) was considered and **deferred** — see
Consequences for why the deployment can't host one safely yet. ForwardEmail's
inbound is **polled**, exactly like Fastmail's.

## Decision

A **delivery mechanism** is a first-class, named provider that may contribute an
**outbound sender** (implements `Mail.Submit(raw)`) and/or an **inbound source**
(feeds the existing `route()` → `route-email`, [04](./04-email.md)). Fastmail,
ForwardEmail, and the spool are mechanisms.

**Construct every mechanism's edges at boot; let the model decide whether and how
each is used.** No mechanism is started or stopped at runtime — only gated.

- **Outbound is a dispatcher.** Every sender is built at boot, each holding its own
  credential reference, in a `senders map[string]Mail` on the send edge. The
  `send-outbox` handler — which reads the live model — resolves `OutboundVia` to a
  mechanism *name* and threads that name into the `send-email` payload; the effect
  calls `senders[name].Submit(raw)`. Effects still never read the model, and no new
  `Secrets` edge is added — each sender resolves its own credential the way
  `NewJMAP` does today. Resolving in the handler (not the reconcile subscription)
  keeps the choice current if `OutboundVia` changes while a row is queued.
- **Inbound is polled, like Fastmail.** ForwardEmail delivers over IMAP; käsi polls
  it as the Fastmail poller polls JMAP, feeding the same `route()` → `route-email`
  pipeline. Retry-safety is the existing `route-email` idempotency on `InboxID`
  ([04](./04-email.md)) — the poller re-emits unconditionally and the handler drops
  an already-ingested inbox row; no new dedupe machinery. **No public endpoint:**
  the mechanism inherits Fastmail's host-gated posture.
- **Each poller is model-gated.** Fastmail's and ForwardEmail's pollers both check
  per tick whether their inbound source is enabled in the model, and no-op
  otherwise. The poll cursor stays in the log
  ([decision-018](./decision-018-poll-cursor-in-the-log.md)).

Mechanism configuration is **model state owned by the email module**
(`email.Model.Mechanisms`, `email.Model.OutboundVia`), logged and replayable
([decision-001](./decision-001-ui-request-is-a-model-entry-not-a-content-table.md)).
Credentials are **secrets**: a provider's API token / IMAP password is written at
the web edge to the secrets store and referenced as `secret://…`, never in the
model or log
([decision-004](./decision-004-secrets-are-written-at-the-web-edge-resolved-at-the-agent-edge.md)).

It is **set up through the settings UI**
([decision-020](./decision-020-settings-are-typed-contributions-rendered-by-a-runtime-form-engine.md)):
the email module contributes a **flat** ForwardEmail setting group — domain, IMAP
host, the API token / IMAP password (a `secret` field), and `inbound`/`outbound`
toggles. It is the settings engine's first **secret-bearing** setting, so building
it requires implementing decision-020's decision-004 secret gate, which is a no-op
today (no prior setting carried a secret). The form is **flat** — no shape-changing
action — so it stays clear of decision-020's rule that a secret must not ride a
dynamic reshape.

**Safety is configuration-gated, not launch-gated.** A fresh boot stays safe by
default — spool out, no live inbound — and the `-poll`/`-send`/`-from` flags remain
as that default and as a dev/bootstrap escape hatch. A mechanism is **inert until
deliberately configured**: ForwardEmail cannot send until its API token is stored
*and* `outbound` is on, and cannot receive until its IMAP password is stored and
`inbound` is on. Enabling **outbound** additionally requires a deliverable
reply-from and a reachable base-url — the check the `-send` boot guard performs
today (`cmd/kasi/serve.go`) moves into the outbound-enable path, so turning a sender
on in the UI cannot start sending mail nobody receives. This deliberately revises
decision-020's "`poll`/`send` stay flags for launch-enforced safety" for the
configured-mechanism case: the guarantee moves from "the process can't" to "an
unconfigured mechanism can't."

## Consequences

- The email edge stops being boot-frozen and becomes a set of pluggable mechanisms;
  exe.dev's Maildir and a raw-SMTP sender slot into the same abstraction later.
- **No new public surface.** ForwardEmail is polled, so it inherits Fastmail's
  host-gated posture ([decision-006](./decision-006-browse-ui-is-host-gated-no-app-tokens.md))
  — the model-gating adds config, not exposure.
- It proves the settings module's **first secret-bearing setting**, which forces
  decision-020's decision-004 gate (a no-op today) to be built.
- `OutboundVia` selects **one** active sender; multi-sender fallback/priority is
  deferred.
- Effects still never read the model, and the reducer never manages a goroutine or
  a route — the architecture is untouched; only the model gained the config and the
  edges gained a gate.
- ForwardEmail inbound over **IMAP** is net-new: käsi speaks JMAP today, not IMAP
  ([04](./04-email.md) notes IMAP as an unbuilt fallback). Polling, cursor, and the
  `route()` reuse follow the Fastmail pattern.

### Deferred: the inbound webhook (real-time push)

A webhook — ForwardEmail POSTing each message to käsi the instant it arrives —
would be faster than polling, but it is **not buildable safely on this deployment**
and is deferred until two problems are solved:

- **Public ingress.** käsi sits private behind exe.dev's IAM edge, which
  authenticates every request to the VM's single public port
  ([decision-006](./decision-006-browse-ui-is-host-gated-no-app-tokens.md)).
  ForwardEmail is not an IAM principal, so its POST is either bounced to a login
  (unreachable) or the port is made public — exposing the *entire* host-gated UI
  (`/settings` holding the API token, `/control`, `/memory`, `/store`) to the
  internet. There is no second public port for a separate listener. A safe public
  ingress for an unauthenticated external caller is an unsolved deployment question.
- **Authentication.** The authenticator must be ForwardEmail's **HMAC webhook
  signature**, verified against a shared secret in the secrets store — *not* a token
  published in a public DNS `TXT` record (world-readable, hence not a secret), and
  the handler must *not* trust payload-reported DKIM/SPF/ARC as an auth gate (an
  attacker forging a POST controls those fields and the `From`, and the initiator
  allowlist does not bound the damage if `From` is spoofed to an allowlisted owner).

Until both hold, polling is the inbound path.

## Related

- [decision-020](./decision-020-settings-are-typed-contributions-rendered-by-a-runtime-form-engine.md) — the settings UI this rides on; this revises its poll/send flag stance for configured mechanisms, and is its first secret-bearing setting.
- [decision-018](./decision-018-poll-cursor-in-the-log.md) — the poll cursor, shared by both polled inbound sources.
- [decision-004](./decision-004-secrets-are-written-at-the-web-edge-resolved-at-the-agent-edge.md) — the API-token / IMAP-password secret path.
- [decision-006](./decision-006-browse-ui-is-host-gated-no-app-tokens.md) — the host-gated posture this upholds (and the wall the deferred webhook hits).
- Chapters [04](./04-email.md) (email edge), [16](./16-settings.md) (settings). Source: `email/module.go`, `email/mail.go`, `email/message_send_outbox.go`, `email/command_send_email.go`, `email/subscription_outbox_reconcile.go`, `cmd/kasi/serve.go`, `web/form_setting.go`.
