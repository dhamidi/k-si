# 04 — Email & routing

Email is käsi's primary interface. All of it flows through a single Fastmail
account over **JMAP**. This document covers inbound delivery, address-based
routing, outbound sending, and threading.

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
3. **Announce.** Emit an `email.received` runtime message referencing the inbox
   row id and recipient. This is the point where durable content becomes a
   logged event ([01](./01-architecture.md)).
4. **Route.** The `email.received` handler inspects the recipient's **local
   part** and selects a route (below), then either threads the mail onto an
   existing task or creates a new one.

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
  the user answering the agent's question — and is appended to that task
  ([05](./05-agents-and-tasks.md)). Otherwise the local part selects a template
  and a **new task** is created.
- **Sender allow-listing.** Because the domain is catch-all, anyone could email
  it. käsi acts only on mail from allow-listed senders (the owner's addresses),
  configured in the UI. Non-allow-listed mail is stored (`status='ignored'`) but
  produces no task. This is käsi's spam/abuse boundary; it is not
  authentication (there is none in-app — see [08](./08-web-ui.md)).
- **Unknown local part.** Falls through to the default route (the general main
  agent), rather than being rejected.

## Outbound: from agent to reply

When a task produces a response ([05](./05-agents-and-tasks.md)):

1. A handler emits a command to **assemble** a MIME reply from the agent's `out/`
   ([02](./02-object-model.md)): body text, harvested attachments, and threading
   headers.
2. The assemble effect writes a `pending` row to the `outbox`
   ([03](./03-persistence.md)) and emits `email.queued`.
3. A **send** subscription/command transmits every `pending` outbox row via JMAP
   (`Email/set` + `EmailSubmission/set`), then emits `email.sent`, whose handler
   marks the row `sent`.

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
