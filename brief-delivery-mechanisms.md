# Brief — delivery mechanisms (ForwardEmail + the pluggable email edge)

Actions to build [decision-023](docs/decision-023-delivery-mechanisms-are-configured-in-the-model.md).
Rides on the settings engine ([decision-020](docs/decision-020-settings-are-typed-contributions-rendered-by-a-runtime-form-engine.md)) —
its `settings/` package, reshape, and base-url migration (`admin.BaseURLOf`) are
built, but its **decision-004 secret gate is a no-op today** and no former yields a
`group`; both are prerequisites here (§4). ForwardEmail inbound is **polled over
IMAP** — no webhook (deferred, see end). Spec only; no code exists yet.

## The mechanisms today and after

| Mechanism | Outbound | Inbound | Config | Credential |
|---|---|---|---|---|
| `spool` | writes `.eml` files | — | boot default | none |
| `fastmail` | JMAP `Email/set`+`EmailSubmission/set` | poll `Email/changes` | model | `secret://fastmail/api-token` |
| `forwardemail` | REST `POST /v1/emails` (or SMTP :465) | **poll IMAP** `imap.forwardemail.net` | model | `secret://forwardemail/api-token` + `secret://forwardemail/imap-password` |

Later: `exedev-maildir` (inotify on `~/Maildir/new`), `smtp` (raw). Same abstraction.

## 0. Model (email module owns it)

- [ ] `email/model_email.go` — add `Mechanisms map[string]Mechanism` and `OutboundVia string`. `Mechanism{Inbound, Outbound bool; Domain string; SendCredRef, RecvCredRef string /* secret:// */}`.
- [ ] `email/msg/` — `set-mechanism` (upsert one mechanism's config), `set-outbound-via` (choose the active sender). Complete imperative messages, logged/replayable.
- [ ] Readers: `OutboundVia(v)`, `Mechanism(v, name)`, `InboundEnabled(v, name)`. Pure, exported (docs/15).
- [ ] No minted webhook token — there is no webhook. Credentials are all secrets (§4).

## 1. Outbound dispatcher (effects stay model-blind; minimal machinery)

- [ ] `email/mail.go` — keep `Mail interface { Submit(ctx, raw) error }`. Build one `Mail` per mechanism at boot, each resolving its own cred ref the way `NewJMAP(sec, ref)` does. Hold them in `senders map[string]Mail` on the send edge. **No new `Secrets` edge on `email`.**
- [ ] `email/message_send_outbox.go` — this handler reads the live model: resolve `OutboundVia` to a mechanism **name** and thread it into the `send-email` payload (`{Mechanism string}`). Resolve here, NOT in `subscription_outbox_reconcile.go` (which captures the entry at sub-build time and would use a stale `OutboundVia`).
- [ ] `email/command_send_email.go` — `senders[p.Mechanism].Submit(raw)`. Never reads the model. Unknown/disabled mechanism → leave the row `pending` (retried once per restart, see note) — don't drop.

## 2. Inbound over IMAP (ForwardEmail as a polled source, like Fastmail)

- [ ] `email/imap.go` — a minimal IMAP client (net-new; käsi speaks JMAP only today). Fetch new messages since the cursor; return raw RFC-5322 + envelope recipient + Message-ID.
- [ ] `cmd/kasi/serve.go` — a second poller goroutine for ForwardEmail, structured like `pollInbox`, calling the same `route()`. Per tick, read `email.InboundEnabled(app.View(), "forwardemail")`; no-op when off. Cursor (IMAP UIDVALIDITY/UID) rides the log like the JMAP state (decision-018).
- [ ] Reuse `route()` verbatim: `content.AddInbox(...)` then emit `route-email` **unconditionally**. Retry-safety is `route-email`'s idempotency on `InboxID` (`tasks.HasIngestedInbox`, `email/message_route_email.go`) — the existing poll path already relies on this. Do NOT invent a Message-ID dedupe branch (`AddInbox` returns the existing id but does not signal duplicate-vs-new).
- [ ] Gate the existing Fastmail poller the same way (`InboundEnabled(…, "fastmail")`).

## 3. (No inbound webhook — deferred)

See "Deferred" at the end. No public route, no token, no `web/server.go` change for inbound.

## 4. Set it up through käsi — the settings customer (build the secret gate first)

- [ ] **Prerequisite:** implement decision-020's decision-004 **secret gate** in `web/form_setting.go` (a no-op today): a `secret`-kind field is written to `secrets.Set` → `secret://…` at the web edge and substituted as a reference *before* `Form.Parse`; plaintext never enters the model, the log, or a re-render.
- [ ] **Prerequisite:** a `group`-kind former (none exists) so a mechanism renders as a flat set of labelled fields.
- [ ] `email/settings.go` — contribute a **flat** `forwardemail` group Setting (short/long descriptions): `domain` (text), `api_token` (**secret**), `imap_password` (**secret**), `inbound` (bool), `outbound` (bool). **No shape-changing action** — flat only, so the secret fields never ride a reshape (decision-020's secret×dynamic rule).
- [ ] On save: emit `set-mechanism{forwardemail, …}`. Enabling `outbound` as the active sender also emits `set-outbound-via`.
- [ ] **Outbound deliverability guard:** enabling `outbound` requires a resolvable reply-from + base-url (the check `cmd/kasi/serve.go:71` does for `-send` today) — validate at save time and reject otherwise, so a UI toggle can't start sending undeliverable mail.

## 5. ForwardEmail sender (the `Mail` twin)

- [ ] `email/forwardemail.go` — `NewForwardEmail(secrets, sendCredRef)`; `Submit(ctx, raw)` does REST `POST https://api.forwardemail.net/v1/emails` (Basic auth `API_TOKEN:`) or SMTP `smtp.forwardemail.net:465`. The assembled RFC-5322 already carries `From`/`In-Reply-To`/`References` (docs/04); ForwardEmail derives the **DKIM signing domain from `From`**, so the sender needs no `domain` argument.
- [ ] Confirm ForwardEmail's paid tier is required for both sending and IMAP inbox.

## 6. serve.go wiring

- [ ] Build all senders at boot into the `senders` map; pass it to the email module edges. `spool` is the default `OutboundVia` on a fresh model.
- [ ] Boot flags become **guarded seeds** (decision-020 only-if-unset, matching `seedAllowlist` — NOT the current unconditional re-sends): `-send` → seed `fastmail.Outbound`+`OutboundVia=fastmail` if unset; `-from` → `set-reply-from` if unset; `-poll` → `fastmail.Inbound` if unset.

## 7. Tests (rings per docs/13)

- [ ] `t/mail/mechanism-outbound-dispatch.test` — `OutboundVia` selects the backend; unknown mechanism leaves the row pending, no drop.
- [ ] `t/mail/forwardemail-poll-inbound.test` — IMAP poll → `route()` → task; a re-polled (same `InboxID`) message creates no second task.
- [ ] `t/web/settings-forwardemail.test` — `visit /settings`, set the flat group; the two secrets go to the store (not the model/log); `visit` render assertion (decision-008). Enabling outbound with no reply-from is rejected.
- [ ] Ring-3 live probe (spends real mail) — behind the existing `probe` gate.

## Notes / deferred

- **"Pending" recovery is once-per-restart.** A pending outbox row gets a one-shot `Await` sub; the runtime keeps a completed sub and won't re-fire it in-process (`runtime/loop.go`). A poison row (disabled/missing-secret mechanism) re-attempts only on restart — it does not block other rows (each has its own sub). Consider a bounded retry / dead-letter later.
- **Multi-sender fallback/priority** — one `OutboundVia` at a time.
- **exe.dev Maildir + raw SMTP** mechanisms — same abstraction, later.
- **The inbound webhook (real-time push) is deferred** (decision-023): it needs a public ingress this deployment can't offer safely (an unauthenticated external POST vs the IAM-gated single public port) *and* real HMAC signature auth (a stored secret, not a DNS-published token; do not trust payload-reported DKIM). Until both exist, IMAP polling is the inbound path.
