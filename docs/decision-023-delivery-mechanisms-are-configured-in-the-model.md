# Decision 023 — delivery mechanisms are configured in the model, edges stay static

## Context

Chapter [04](./04-email.md) describes an email edge that is frozen at boot. There
is exactly one outbound `Mail` (a spool in development, Fastmail JMAP with `-send`)
and one inbound source (the Fastmail poller, gated by `-poll`). *Which* provider,
and whether it is live, are launch flags decided before the log is even open.

We want to add **ForwardEmail** as a *separate* provider — coexisting with
Fastmail, each enabled on its own — and, crucially, to **set it up through käsi's
web UI** without a redeploy: enter the credential, get a DNS record to paste, flip
it on. More generally, we want the email edge to be pluggable (Fastmail,
ForwardEmail, spool today; exe.dev Maildir, raw SMTP later).

Two forces collide. Configuration that is set up through käsi belongs in the model
and must take effect at runtime — that is the whole point of the settings module
([decision-020](./decision-020-settings-are-typed-contributions-rendered-by-a-runtime-form-engine.md)).
But edges are constructed once, in `main.go`, and the reducer never starts a
goroutine or swaps an edge ([01](./01-architecture.md)). In fact decision-020 kept
`-poll`/`-send` as flags *precisely because they activate edges*. This decision
resolves that tension for a configured provider.

## Decision

A **delivery mechanism** is a first-class, named provider that may contribute an
**outbound sender** (implements `Mail.Submit(raw)`) and/or an **inbound source**
(feeds the existing `route-email`, [04](./04-email.md)). Fastmail, ForwardEmail,
and the spool are mechanisms.

**Construct every mechanism's edges at boot; let the model decide whether and how
each is used.** No mechanism is started or stopped at runtime — only gated.

- **Outbound is a dispatcher.** Every sender is built at boot. The outbox-reconcile
  subscription — which already reads the model — reads `OutboundVia` and the chosen
  mechanism's credential reference, and threads `{mechanism, credRef}` into the
  `send-email` payload. The effect resolves the secret at the edge and calls the
  selected backend. Effects still never read the model; data reaches them by
  payload ([01](./01-architecture.md)).
- **The inbound webhook is a static route.** `POST /inbound/{mechanism}/{token}` is
  mounted at boot. Its handler reads the model (web handlers may — [08](./08-web-ui.md))
  to confirm the mechanism is enabled and to validate the token, then does the
  ordinary `route()` work (store raw to `inbox`, emit `route-email`).
- **The poller is model-gated.** The Fastmail poller stays, but checks per tick
  whether its inbound source is enabled and no-ops otherwise.

Mechanism configuration is **model state owned by the email module**
(`email.Model.Mechanisms`, `email.Model.OutboundVia`), logged and replayable
([decision-001](./decision-001-ui-request-is-a-model-entry-not-a-content-table.md)).
Credentials split by kind: a provider **API token is a secret**
(`secret://…`, written at the web edge, never in the model or log —
[decision-004](./decision-004-secrets-are-written-at-the-web-edge-resolved-at-the-agent-edge.md)),
while the inbound **webhook token is a minted capability value** stored in the
model, exactly as completion tokens are
([decision-003](./decision-003-request-links-mirror-the-completion-link-keyed-by-run-id.md)).

It is **set up through the settings UI**
([decision-020](./decision-020-settings-are-typed-contributions-rendered-by-a-runtime-form-engine.md)):
the email module contributes a structured (`group`) ForwardEmail setting whose
`secret` field (the API token) flows through decision-020's decision-004 gate, and
whose enable action **mints the webhook token and renders the DNS `TXT` record** to
paste at the registrar. This is the first real customer of the settings engine — a
structured, secret-bearing, generated-value setting.

**Safety is configuration-gated, not launch-gated.** A fresh boot stays safe by
default — spool out, no live inbound — and the `-poll`/`-send`/`-from` flags remain
as that default and as a dev/bootstrap escape hatch. A real mechanism is **inert
until deliberately configured**: ForwardEmail cannot send until its API token is
stored *and* `outbound` is on, and the webhook rejects until the mechanism is
enabled with a matching token. This deliberately revises decision-020's
"`poll`/`send` stay flags for launch-enforced safety" for the configured-mechanism
case: the guarantee moves from "the process can't" to "an unconfigured mechanism
can't."

## Consequences

- The email edge stops being boot-frozen and becomes a set of pluggable mechanisms;
  exe.dev's Maildir and a raw-SMTP sender slot into the same abstraction later.
- The inbound webhook is a **new public surface** — the deliberate exception to the
  host-gated posture ([decision-006](./decision-006-browse-ui-is-host-gated-no-app-tokens.md)),
  in the spirit of the control endpoint
  ([decision-014](./decision-014-notifications-are-fire-and-forget.md)). It is
  guarded by an unguessable token in the URL, re-runs the initiator allowlist,
  checks the DKIM/SPF/ARC results carried in the payload, and **returns 200 only
  after the `inbox` row is committed** (the durable inbox), deduped by `Message-ID`
  so a provider retry is harmless.
- It proves the settings module end to end, which is the point of building it now.
- `OutboundVia` selects **one** active sender; multi-sender fallback/priority is
  deferred.
- Effects still never read the model, and the reducer never manages a goroutine or
  a route — the architecture is untouched; only the model gained the config and the
  edges gained a gate.

## Related

- [decision-020](./decision-020-settings-are-typed-contributions-rendered-by-a-runtime-form-engine.md) — the settings UI this rides on; this revises its poll/send flag stance for configured mechanisms.
- [decision-018](./decision-018-poll-cursor-in-the-log.md) — the Fastmail poller, now one gated inbound source among several.
- [decision-004](./decision-004-secrets-are-written-at-the-web-edge-resolved-at-the-agent-edge.md) — the API-token secret path; [decision-003](./decision-003-request-links-mirror-the-completion-link-keyed-by-run-id.md) — the minted webhook token.
- [decision-006](./decision-006-browse-ui-is-host-gated-no-app-tokens.md), [decision-014](./decision-014-notifications-are-fire-and-forget.md) — the host-gated posture and the sibling public control surface.
- Chapters [04](./04-email.md) (email edge), [16](./16-settings.md) (settings). Source: `email/module.go`, `email/mail.go`, `email/command_send_email.go`, `email/command_assemble_reply.go`, `cmd/kasi/serve.go`, `web/server.go`.
