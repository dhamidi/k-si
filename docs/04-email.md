# 04 — Email & routing

Email is käsi's primary interface. It flows through one or more **delivery
mechanisms** — pluggable providers, each contributing inbound, outbound, or both,
configured from the web UI rather than wired at boot
([decision-023](./decision-023-delivery-mechanisms-are-configured-in-the-model.md)).
Fastmail over **JMAP** is the built-in mechanism and the running example
throughout. This document covers inbound delivery, address-based routing, outbound
sending, threading, and how mechanisms plug in.

## Delivery mechanisms

Mail reaches käsi, and leaves it, through a **delivery mechanism** — a named
provider that contributes an outbound sender, an inbound source, or both. Fastmail
(below) is one; ForwardEmail is another; the development spool is a third. You add
and configure them from the settings UI, not by redeploying
([16](./16-settings.md),
[decision-023](./decision-023-delivery-mechanisms-are-configured-in-the-model.md)).

Two rules keep this from fighting boot-time assembly ([01](./01-architecture.md)):

- **Every mechanism is built at boot; the model decides which are used.** Nothing
  starts or stops a goroutine or swaps an edge at runtime — a mechanism is *gated*,
  not lifecycled. The outbound path is a dispatcher that sends through whichever
  mechanism `OutboundVia` names; the inbound webhook route is always mounted but
  only accepts when its mechanism is enabled and its token matches; the poller
  checks per tick whether it is on.
- **Configuration is model state; credentials are references.** Which mechanisms
  are enabled, which one sends, and each provider's domain live in the email model,
  logged and replayable. A provider's API token is a secret (`secret://…`,
  [06](./06-secrets.md)); an inbound webhook token is a minted capability value
  stored in the model, like the completion token.

A mechanism is **inert until configured** — it can neither send nor receive until
its credential is stored and it is switched on — so a fresh install sends nothing
by accident, and enabling real mail is a deliberate act. The `-poll` / `-send` /
`-from` flags remain only as the safe boot default and a dev escape hatch.

**Inbound over a webhook.** A mechanism may deliver mail by POSTing it to käsi
rather than being polled. The route `POST /inbound/{mechanism}/{token}` validates
the token, re-runs the authorisation gates below, stores the raw MIME to `inbox`
**before it answers 200**, then emits `route-email` exactly as the poller does —
deduped on `Message-ID` so a provider's retry is harmless. This is the one public
entry point in an otherwise host-gated deployment ([08](./08-web-ui.md)); the
unguessable token in its URL is what guards it.

**ForwardEmail** is the first webhook mechanism: enter its API token and domain in
the settings UI, käsi mints the webhook token and shows the DNS `TXT` record to
paste (`forward-email=https://…/inbound/forwardemail/<token>`), and mail flows —
inbound over the webhook, outbound over ForwardEmail's API with DKIM on your
domain. See the feature guide *Delivery mechanisms* for the walkthrough.

## Fastmail over JMAP

[JMAP](https://jmap.io/) is Fastmail's modern JSON-over-HTTPS API. käsi uses it
for both directions, authenticated with a **Bearer API token** (created in
Fastmail's *Privacy & Security* settings) stored as a secret
([06](./06-secrets.md)).

- **Session discovery:** `GET https://api.fastmail.com/jmap/session` returns the
  account id and the API/download/upload endpoints. Do this once at startup.
- **Receiving:** `Email/query` + `Email/get` to pull messages;
  `Email/changes` to sync incrementally by state.
- **Sending:** `Email/set` (create the draft) followed by
  `EmailSubmission/set` (submit it), requiring the
  `urn:ietf:params:jmap:submission` capability.
- **Attachments:** binary parts are uploaded to the JMAP upload endpoint and
  referenced by blobId when building the email; downloaded via the download
  endpoint.

We speak JMAP directly with the standard-library HTTP client and JSON — no SDK.
IMAP/SMTP remain as a fallback path but JMAP is the primary integration.

## Inbound: from mailbox to task

The domain **`kasi.decode.ee`** is configured in Fastmail so that *all* mail to
any local part is delivered to käsi's account (a catch-all on the custom
domain). This is what makes `pay@`, `research@`, `anything@kasi.decode.ee`
addressable without pre-registering each address at the mail provider.

The pipeline:

1. **Detect.** An inbox subscription learns about new mail — by polling
   `Email/changes` on an interval, or by holding a JMAP push/EventSource
   connection where available. Either way it produces work; polling is the
   simple, always-available baseline.
2. **Fetch & store.** For each new message, fetch the raw RFC 5322 bytes and
   insert a row into the `inbox` table ([03](./03-persistence.md)) with the
   envelope recipient recorded. Storage is idempotent on `Message-ID`.
3. **Announce.** Emit a `route-email` runtime message that carries the routing
   facts inline — recipient, sender, `Cc` list, subject, `Message-ID`,
   `In-Reply-To`/`References` — and references the inbox row id for the bulk MIME.
   The message is imperative and **complete**: the handler routes purely from it,
   without re-opening the stored mail ([01](./01-architecture.md)). This is the
   point where durable content becomes a logged event.
4. **Route.** The `route-email` handler inspects the recipient's **local part**,
   checks authorisation (below), and hands off to the tasks domain via the
   built-in `send` command ([01](./01-architecture.md)): `append-to-task` to
   thread the mail onto an existing task, `create-task` to start a new one.

Storing to SQLite *before* emitting the runtime message means a crash between
"Fastmail has the mail" and "käsi processed it" cannot lose the mail: on restart
the inbox row is still there, and the poller's high-water mark (a JMAP state
string, itself carried on logged messages) resumes correctly.

## Routing by local part

The **local part** of the recipient address is the router key.

```
pay@kasi.decode.ee        ->  route "pay"       ->  invoice-payment template
research@kasi.decode.ee   ->  route "research"  ->  research template
<anything else>@…         ->  default route     ->  main/general template
```

A **route** maps a local part to a **task template** — the prompt, skills, and
tools that define that category of work ([02](./02-object-model.md),
[07](./07-skills-and-tools.md)). Routes are configuration held in the model and
edited from the web UI ([08](./08-web-ui.md)); adding a new capability is
"define a template, bind a local part," no code change and no mail-provider
change.

Routing decisions:

- **Thread vs. new task.** If the message carries `In-Reply-To` / `References`
  matching a known task's thread key, it is a *reply within an existing task* —
  someone answering the agent's question — and is appended to that task
  ([05](./05-agents-and-tasks.md)). Otherwise the local part selects a template
  and a **new task** is created.
- **Unknown local part.** Falls through to the default route (the general main
  agent), rather than being rejected.

### Authorisation: who may act, and on what

Because the domain is catch-all, anyone could email it. käsi acts only on
authorised mail. There are **two distinct gates**, and a message must pass the one
that applies to it:

1. **Initiating a new task** requires the sender to be on the **initiator
   allowlist** — a global list of addresses (the owner's) permitted to *start*
   conversations, configured in the UI ([08](./08-web-ui.md)). A new-task email
   from an address not on the list is stored (`status='ignored'`) and produces no
   task. This is käsi's spam/abuse boundary; it is not authentication (there is
   none in-app — see [08](./08-web-ui.md)).

2. **Interacting with an existing task** requires the sender to be a
   **participant** of *that task* — either the initiator or an address that was
   CC'd in by an authorised sender (below). A reply from a non-participant is
   ignored for that task.

### Adding collaborators by CC

Participation is granted dynamically, by email, with no UI visit: **when an
authorised sender CCs an address on a message to käsi, that address becomes a
participant of the task.**

- CC'ing `alice@example.com` on the *first* email (initiator on the allowlist)
  seeds the new task's participant set with Alice.
- CC'ing someone on a *later* reply (from any current participant) adds them to
  the task from then on.
- Participation is **task-scoped**: it lets Alice reply into *this* thread and
  have käsi act on it. It does **not** put Alice on the initiator allowlist — she
  still cannot start new tasks.
- Replies käsi sends go to the task's participants (reply-all across the thread),
  so the conversation stays shared.

Concretely, `route-email` carries the `Cc` list along on the messages it
sends: `create-task` seeds `Task.participants` ([02](./02-object-model.md))
and `append-to-task` / `add-collaborator` extend them — all handled in the
tasks domain, gated by the sender already being authorised for the task. The
allowlist itself is email-domain state, edited via `allow-sender` /
`revoke-sender` messages from the UI. All of this is pure model state over
complete messages ([01](./01-architecture.md)).

## Outbound: from agent to reply

When a task produces a response ([05](./05-agents-and-tasks.md)):

1. A handler emits a command to **assemble** a MIME reply from the agent's `out/`
   ([02](./02-object-model.md)): body text, harvested attachments, and threading
   headers.
2. The assemble effect writes a `pending` row to the `outbox`
   ([03](./03-persistence.md)) and emits `mark-reply-queued`.
3. A **send** subscription/command transmits every `pending` outbox row through
   the active mechanism — JMAP for Fastmail, a REST call for ForwardEmail, a file
   for the spool — selected by `OutboundVia` (see *Delivery mechanisms* above). It
   then emits `mark-email-sent`, whose handler marks the row `sent`.

Because the outbox is a durable, idempotent queue, a crash mid-send is
recoverable: reconciliation re-sends anything still `pending`
([03](./03-persistence.md)), and the pre-generated `Message-ID` guards against
duplicates.

## Threading

Replies must land in the *same* email conversation the user started, so they read
as a normal back-and-forth on their phone.

- The **task's thread key** is the `Message-ID` of the originating email plus the
  accumulated `References` chain.
- Every outbound message sets `In-Reply-To` to the message it answers and
  extends `References` with the full chain.
- `Subject` is preserved (with `Re:` prefixing) so mail clients group the thread.
- `From` is the route address the user wrote to (e.g. `pay@kasi.decode.ee`), so
  replying-to-reply keeps hitting the same route and the same task.

Custom `X-Kasi-Task` / `X-Kasi-Cause` headers ([02](./02-object-model.md)) give
käsi a robust secondary way to associate an inbound reply with its task even if a
client mangles the standard threading headers.

## The completion link

Every reply includes a **"mark this task done"** link — a URL into the web UI
([08](./08-web-ui.md)) carrying the task id and an unguessable token. Clicking it
transitions the task to `done`, which triggers archival and workspace cleanup
([05](./05-agents-and-tasks.md)). This is the one routine interaction that leaves
email for the web, and it is a single click. The token makes the link
capability-bearing so it works without in-app auth.

## Capability links, generally

The completion link is one of two kinds of **capability link** käsi puts in email
replies — a tokenised URL that grants a specific one-off action without any login:

- the **completion link**, which marks the task done (above);
- a **request link**, which opens an agent-raised web form to collect files,
  structured fields, or secrets ([05](./05-agents-and-tasks.md),
  [08](./08-web-ui.md)) — the way an agent asks for input that doesn't belong in
  an email body.

Both carry an unguessable per-action token so they work as one tap from a phone
while the deployment stays private ([08](./08-web-ui.md)). A request link is how
the user avoids pasting a secret into email: the agent asks, the reply carries the
link, and the answer is provided on the web instead.
