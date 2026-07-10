# Brief — delivery mechanisms (ForwardEmail + the pluggable email edge)

Actions to build [decision-023](docs/decision-023-delivery-mechanisms-are-configured-in-the-model.md).
Rides on the settings engine ([decision-020](docs/decision-020-settings-are-typed-contributions-rendered-by-a-runtime-form-engine.md)) —
land or stub the settings `Setting`/`Form`/secret-gate first, since the setup UI is
how a mechanism is configured. Spec only; no code exists yet.

## The mechanisms today and after

| Mechanism | Outbound | Inbound | Config | Credential |
|---|---|---|---|---|
| `spool` | writes `.eml` files | — | boot default | none |
| `fastmail` | JMAP `Email/set`+`EmailSubmission/set` | poller (`Email/changes`) | model | `secret://fastmail/api-token` |
| `forwardemail` | REST `POST /v1/emails` (or SMTP :465) | webhook `POST /inbound/forwardemail/{token}` | model | `secret://forwardemail/api-token` + minted webhook token |

Later: `exedev-maildir` (inotify on `~/Maildir/new`), `smtp` (raw). Same abstraction.

## 0. Model (email module owns it)

- [ ] `email/model_email.go` — add `Mechanisms map[string]Mechanism` and `OutboundVia string` to `Model`. `Mechanism{Inbound, Outbound bool; Domain string; APITokenRef string /* secret:// */; WebhookToken string /* minted */}`.
- [ ] `email/msg/` — `set-mechanism` (upsert one mechanism's config), `set-outbound-via` (choose the active sender). Both are complete imperative messages, logged/replayable.
- [ ] Reader helpers: `OutboundVia(v)`, `Mechanism(v, name)`, `InboundEnabled(v, name)`. Pure, exported (docs/15).
- [ ] Credential split: **API token → secret** (`secret://…`, decision-004, written at the web edge). **Webhook token → minted capability value** in the model (128-bit `mintToken`, like completion tokens, decision-003), displayed once at setup.

## 1. Outbound dispatcher (effects stay model-blind)

- [ ] `email/mail.go` — keep `Mail interface { Submit(ctx, raw) error }`. Add a `Sender` per mechanism (JMAP, ForwardEmail, Spool), all built at boot and held in a `map[string]Mail` on the send edge.
- [ ] `email/subscription_outbox_reconcile.go` (reads the model already) — read `OutboundVia` + that mechanism's `APITokenRef`; thread `{Mechanism, CredRef}` into the `send-email`/`send-outbox` payload.
- [ ] `email/command_send_email.go` — the effect resolves `CredRef` via the `Secrets` edge and calls `senders[p.Mechanism].Submit(raw)`. Never reads the model. Unknown/disabled mechanism → leave the row `pending` (reconcile retries), don't drop.

## 2. Inbound webhook (the one public surface)

- [ ] `web/server.go` — mount `POST /inbound/{mechanism}/{token}` at boot (static route). No app token, but NOT host-gated: this is the deliberate public exception (decision-023), guarded by the URL token.
- [ ] Handler `inboundWebhook`: (1) look up the mechanism in the model (`app.View()`), 403 if `!Inbound`; (2) constant-time compare `{token}` against `Mechanism.WebhookToken`; (3) parse the provider payload → raw MIME + envelope recipient + Message-ID + DKIM/SPF/ARC results; (4) reject on failed auth results; (5) `content.AddInbox(...)` (idempotent on Message-ID) **then** `200` (ack-after-commit); (6) `app.Send(NewRouteEmail(...))` reusing the existing `route()` shape (mint a completion token as today). Duplicate Message-ID → `200` without re-emitting (retry-safe).
- [ ] The existing initiator-allowlist / participant gates run unchanged inside `route-email` — no new authz.

## 3. Poller gating (Fastmail stays, one source among several)

- [ ] `cmd/kasi/serve.go` `pollInbox` — per tick, read `email.InboundEnabled(app.View(), "fastmail")`; no-op when off. The poll cursor stays in the log (decision-018).
- [ ] `-poll` flag: on first boot only, seed `fastmail.Inbound = true` **if unset** (guarded seeding, decision-020 ruling). Thereafter the UI owns it.

## 4. Set it up through käsi (the decision-020 customer)

- [ ] `email/settings.go` — contribute a `forwardemail` **group** Setting (short/long descriptions): fields `domain` (text), `api_token` (**secret** kind → decision-004 gate writes `secrets.Set` → `secret://forwardemail/api-token`, never in model/log), `inbound` (bool), `outbound` (bool).
- [ ] Generated value: a **"generate webhook & DNS record"** form action (ToFormer `Update`) that mints `WebhookToken`, then renders the exact record to copy: `forward-email=https://<base>/inbound/forwardemail/<token>`. `<base>` comes from the (migrated) base-url setting.
- [ ] On save: emit `set-mechanism{forwardemail, …}`; flipping `outbound` on is also `set-outbound-via` when the user makes it the active sender.
- [ ] This is the first structured + secret + generated setting — exercises the group kind, the decision-004 gate, and a generate-value action end to end.

## 5. ForwardEmail sender (the `Mail` twin)

- [ ] `email/forwardemail.go` — `NewForwardEmail(secrets, tokenRef, domain)`; `Submit(ctx, raw)` does REST `POST https://api.forwardemail.net/v1/emails` (Basic auth `API_TOKEN:`), or SMTP `smtp.forwardemail.net:465`. The assembled RFC-5322 already carries `In-Reply-To`/`References`/`From` (threading, docs/04); ForwardEmail auto-adds DKIM for the domain.
- [ ] Verify their retry/bounce policy for the inbound webhook (ack-after-commit assumes retry-until-200). Confirm the inbound payload includes raw + envelope recipient + auth results.

## 6. serve.go wiring

- [ ] Build all senders at boot; pass the `map[string]Mail` + `Secrets` to the email module edges. Keep `spool` as the default `OutboundVia` on a fresh model.
- [ ] Boot flags become guarded seeds (decision-020): `-send` → seed `fastmail.Outbound`+`OutboundVia=fastmail` if unset; `-from` → `set-reply-from` if unset; `-poll` → `fastmail.Inbound` if unset.

## 7. Tests (rings per docs/13)

- [ ] `t/mail/mechanism-outbound-dispatch.test` — `OutboundVia` selects the backend; unknown mechanism leaves the row pending.
- [ ] `t/mail/forwardemail-inbound.test` — webhook POST → `route-email`; bad token → 403; duplicate Message-ID → 200, no second task; committed to `inbox` before 200.
- [ ] `t/web/settings-forwardemail.test` — `visit /settings`, set the group, secret goes to the store (not the model), the DNS record renders with the minted token (decision-008 `visit` assertion).
- [ ] Ring-3 live probe (spends real mail) — behind the existing `probe` gate.

## Out of scope / deferred

- Multi-sender fallback/priority (one `OutboundVia` at a time).
- exe.dev Maildir + raw SMTP mechanisms (same abstraction, later).
- Inbound webhook signature verification (rely on the URL token + DKIM results until ForwardEmail signs).
