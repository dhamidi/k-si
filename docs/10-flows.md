# 10 — End-to-end flows

These walkthroughs trace concrete scenarios as sequences of **runtime messages**
(`Msg`) and **commands** (`Cmd`) through the runtime ([01](./01-architecture.md)).
They tie the other documents together and double as a specification of the
happy paths.

Recall the message discipline ([01](./01-architecture.md)): every message is
**imperative** (it tells the model what to do) and **complete** (it carries
everything its handler needs, so the handler does no I/O). Tags below are
illustrative.

Notation: `Msg` in **bold**, `Cmd` in `code`, effects italicised.

## Flow A — Pay an invoice (the canonical example)

You forward an email with a PDF invoice to **`pay@kasi.decode.ee`**, CC your
accountant `alice@example.com`, and get a reply thread until the invoice is paid.

### 1. Delivery → inbox

Fastmail's catch-all on `kasi.decode.ee` delivers the mail to käsi's account
([04](./04-email.md)). The **inbox-poll subscription** notices it via
`Email/changes`, fetches the raw MIME, and writes an `inbox` row
([03](./03-persistence.md)).

→ emits **`route-email`**
`{inbox_id, recipient: "pay@kasi.decode.ee", sender: "you@…", cc: ["alice@…"],
subject, message_id, in_reply_to: null}`

The message is **complete** — it carries the routing facts inline — and is logged
before it is applied ([01](./01-architecture.md)).

### 2. Route → authorise → hand off to tasks

The **`route-email`** handler (email domain), purely from the message and model:

- no `in_reply_to` match → this is a **new task**, so the sender must be on the
  **initiator allowlist** ([04](./04-email.md)) — they are;
- reads local part `pay` → route `pay` → template `invoice-payment`
  ([04](./04-email.md), [07](./07-skills-and-tools.md)).

Routing is email's whole competence; the task itself is `tasks/`' business, so
the handler crosses the domain boundary the only sanctioned way
([01](./01-architecture.md)):

→ returns `[send]` carrying **`create-task`** `{inbox_id, route: pay, template:
invoice-payment, sender, cc, subject, thread keys}` — logged like any message.

The **`create-task`** handler (tasks domain):

- seeds the task's **participants** from sender + `cc` → you and Alice
  ([04](./04-email.md)); Alice may now reply into this thread, but is not added to
  the initiator allowlist.

→ updates model: new `Task{status: open, route: pay, template: invoice-payment,
participants: [you, alice]}` (id derived from the log offset —
[01](./01-architecture.md))
→ returns `[create-workspace, lay-in-inputs, provision-workspace, spawn-agent-run]`

### 3. Prepare the workspace

Effects run concurrently in workers ([01](./01-architecture.md)):

- `create-workspace` — *makes `$WORKDIR/task-$ID/` with `in/` and `out/`*
  ([05](./05-agents-and-tasks.md)).
- `lay-in-inputs` — *writes the email text to `in/body.txt` and the PDF part to
  `in/invoice.pdf`* ([02](./02-object-model.md)).
- `provision-workspace` — *lays the template's skills into `skills/`, writes
  `.mise.toml`, `mise trust`s the workspace, and `mise install`s the pinned tools
  into the shared, persistent mise data dir* ([07](./07-skills-and-tools.md)).

### 4. Run the agent

- `spawn-agent-run` — resolves the run's `secret://` needs at the edge
  ([06](./06-secrets.md)), *starts the Claude harness in the workspace*
  ([05](./05-agents-and-tasks.md)), and registers an **agent-watch
  subscription** for this task.

→ updates model: `Task.status = awaiting-agent`

The reducer is not blocked; the harness runs in its worker while everything else
proceeds ([01](./01-architecture.md)).

### 5. Agent finishes, needs confirmation

The invoice is for a large amount, so the agent decides to confirm before paying.
It writes a question into `out/reply.txt` and exits. The **agent-watch
subscription** sees the process exit and reads what it left behind.

→ emits **`finish-agent-run`**
`{task_id, run_id, exit: 0, transcript_path, out_manifest: ["reply.txt"]}`

The message is complete — the manifest names what's in `out/`, so the handler
decides next steps without touching disk. It returns
`[capture-transcript, assemble-reply]`:

- `capture-transcript` — *copies the session transcript into `archive`*
  (`kind='transcript'`) ([03](./03-persistence.md), [05](./05-agents-and-tasks.md)).
- `assemble-reply` — *harvests `out/` into MIME parts, builds the reply (body =
  the question, threading headers, recipients = participants, a completion link),
  writes a `pending` `outbox` row* ([02](./02-object-model.md), [04](./04-email.md)).

→ emits **`mark-reply-queued`** → handler sets `Task.status = awaiting-user`.

### 6. Send the reply

The **send** path transmits the `pending` outbox row via JMAP
`EmailSubmission/set` ([04](./04-email.md)); the Fastmail token is resolved from
`secret://fastmail/api-token` at the edge ([06](./06-secrets.md)).

→ emits **`mark-email-sent`** `{outbox_id}` → handler marks the row `sent`.

You and Alice both receive the agent's question **as a reply in the original
thread**.

### 7. A participant replies with confirmation

Alice replies "yes, pay it" in the same thread. Steps 1–2 repeat, but now the
**`route-email`** handler matches the thread key → **reply within the existing
task**, and Alice is a **participant**, so she is authorised
([04](./04-email.md), [05](./05-agents-and-tasks.md)):

→ returns `[send]` carrying **`append-to-task`** `{task_id, inbox_id, sender,
cc, …}` — the same cross-domain hand-off as step 2.

The **`append-to-task`** handler (tasks domain):

→ returns `[lay-in-inputs (new text into in/), spawn-agent-run (resume session)]`
→ `Task.status = awaiting-agent`

The agent **resumes the same session** ([05](./05-agents-and-tasks.md)), pays the
invoice using its provisioned tool and `secret://route/pay/*` credential
([06](./06-secrets.md), [07](./07-skills-and-tools.md)), and writes a confirmation
plus `out/receipt.pdf`.

→ **`finish-agent-run`** → `capture-transcript`, `assemble-reply`
(body = "Paid. Receipt attached.", attachment = the receipt) →
**`mark-reply-queued`** → **`mark-email-sent`**.

Everyone on the thread gets the receipt.

### 8. Mark it done

A participant clicks the **completion link** in the reply ([04](./04-email.md),
[08](./08-web-ui.md)). The web edge validates the token and feeds the core
([08](./08-web-ui.md)):

→ emits **`finish-task`** `{task_id}`

The **`finish-task`** handler returns `[archive-task]`:

- `archive-task` — *archives every not-yet-archived `in/` attachment, `out/`
  artifact, and transcript; verifies completeness; then deletes
  `$WORKDIR/task-$ID/`* — archive-then-delete, in that strict order
  ([05](./05-agents-and-tasks.md)).

→ `Task.status = done`. The task lives on entirely in SQLite (emails, artifacts,
transcripts); only the scratch directory is gone.

## Flow B — The clarification loop, abstractly

Flow A steps 5–7 generalise to any multi-turn task:

```
awaiting-agent ──finish-agent-run (asks)──► assemble-reply ──► awaiting-user
      ▲                                                            │
      └──── route-email (participant replies in thread) ─ spawn (resume) ─┘
```

Each turn: one **agent run**, one continuous **session**, one exchange in one
**email thread** — the anchoring equivalence ([05](./05-agents-and-tasks.md)),
sustained for as many rounds as the work needs, until **`finish-task`**.

## Flow C — Agent requests a secret via the web

Mid-task, the pay agent needs a credential it doesn't hold — say a one-time bank
login — and must not ask for it in email. It raises a **UI request** instead
([05](./05-agents-and-tasks.md), [08](./08-web-ui.md)).

- The agent writes `out/request.json` (fields: `bank-login` type `secret`,
  `authorization` type `file`) and `out/reply.txt` ("I need your bank login to
  proceed — please use the secure link below."). It exits.
- **`finish-agent-run`** `{…, out_manifest: ["reply.txt", "request.json"]}` → the
  handler returns `[capture-transcript, mint-ui-request]`.
- `mint-ui-request` — *generates an unguessable token, writes a `pending`
  `ui_request` row ([03](./03-persistence.md)), builds the request link* →
  emits **`register-ui-request`** `{request_id, token, form_spec, link}`.
- **`register-ui-request`** handler adds the request to the model, sets
  `Task.status = awaiting-user`, and drives `assemble-reply` so the emailed reply
  contains the agent's message **and the link** → **`mark-reply-queued`** →
  **`mark-email-sent`**.
- The user taps the link. `GET` renders the form from `form_spec` with htmlc
  ([08](./08-web-ui.md)); the user types the login into a masked field and
  attaches the authorization file, then submits.
- The web edge stores the upload in `archive`, writes the login to the secrets
  database → `secret://task/$ID/bank-login` ([06](./06-secrets.md)), and emits
  **`answer-ui-request`** `{request_id, file_refs:[…], secret_refs:["secret://…"]}`
  — **complete, and reference-only: no plaintext** ([05](./05-agents-and-tasks.md)).
- **`answer-ui-request`** handler marks the request `answered` and returns
  `[lay-in-answers, spawn-agent-run]`. `lay-in-answers` writes the file into `in/`;
  `spawn-agent-run` resolves `secret://task/$ID/bank-login` into the harness
  environment at the edge ([06](./06-secrets.md)) and **resumes the session**.
  `Task.status = awaiting-agent`.
- The agent continues with the credential it needed — which never appeared in an
  email, the log, the model, or a workspace file as plaintext.

## Flow D — Agent authors a reusable skill

During a task, the agent works out how to reconcile a vendor's odd invoice format
and writes the know-how down for next time ([05](./05-agents-and-tasks.md),
[07](./07-skills-and-tools.md)).

- The agent leaves `out/skills/vendor-x-invoices.md` (with metadata). It exits.
- **`finish-agent-run`** `{…, out_manifest: [..., "skills/vendor-x-invoices.md"]}`
  — the manifest flags the authored skill, so the handler adds `store-skill` to
  its returned commands.
- `store-skill` — *writes the skill's content to the `skill` table in SQLite*
  (`origin='agent'`, [03](./03-persistence.md)) — then
  → emits **`register-skill`** `{skill_id, name: "vendor-x-invoices", content_ref}`.
- **`register-skill`** handler adds the skill to the in-RAM registry
  ([07](./07-skills-and-tools.md)).

From now on, `provision-workspace` for any run that includes this skill lays it
into the workspace — the **next turn of this task immediately**, and other tasks
once they reference it. Because it lives in SQLite, it **survives this task's
workspace deletion**: a skill learned once is available to future agent runs.

## Flow E — Crash and restart (durability)

Suppose käsi is killed right after step 6 emits **`mark-email-sent`** but before
Alice replies.

On restart ([01](./01-architecture.md), [03](./03-persistence.md)):

1. Start from the zero model and replay the **entire** `message_log` with
   **effects suppressed** — there are no snapshots ([01](./01-architecture.md)).
   Replaying **`route-email`**, **`create-task`**, **`finish-agent-run`**,
   **`mark-reply-queued`**,
   **`mark-email-sent`** rebuilds the exact model: this task in `awaiting-user`,
   participants intact. No email is re-sent, no agent re-spawned — those were
   effects, and replay performs none.
2. Switch to live mode; compute `subscriptions(model)` and start them, including
   the inbox poller.
3. **Reconciliation** ([03](./03-persistence.md)) inspects the model: the outbox
   row is already `sent`, so nothing to resend; no task is stuck in
   `awaiting-agent` without a live process, so nothing to resume. The system is
   exactly where it left off.

Had the crash happened one step earlier — outbox row `pending`,
**`mark-email-sent`** never logged — replay would rebuild the task with a
`pending` row, and the reconciliation subscription would emit `send-email`,
delivering the reply exactly once (the pre-generated `Message-ID` guards against
duplicates ([04](./04-email.md))). This is why effects are
described-then-interpreted and outbound sending is a durable, idempotent queue.

## What every flow demonstrates

- **Imperative, complete messages → commands → effects → messages**, folded by
  one reducer with no I/O in handlers ([01](./01-architecture.md)).
- **MIME at every boundary** — inbox, `in/`, `out/`, outbox, archive
  ([02](./02-object-model.md)).
- **Secrets resolved only at the edge**, and collected on the web — never in
  email or the log — when an agent requests one ([06](./06-secrets.md),
  [08](./08-web-ui.md)).
- **Durability by full-log replay, not effect-replay and not snapshots**
  ([01](./01-architecture.md), [03](./03-persistence.md)).
- **Capabilities that grow** — participants added by CC, skills authored by
  agents and kept in SQLite, input gathered on demand through agent-raised web
  requests ([04](./04-email.md), [05](./05-agents-and-tasks.md),
  [08](./08-web-ui.md)).
- **One task ⇔ one thread ⇔ one session**, from first email to completion
  ([05](./05-agents-and-tasks.md)).
