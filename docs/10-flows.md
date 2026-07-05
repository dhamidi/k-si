# 10 — End-to-end flows

These walkthroughs trace concrete scenarios as sequences of **runtime messages**
(`Msg`) and **commands** (`Cmd`) through the runtime ([01](./01-architecture.md)).
They tie the other documents together and double as a specification of the
happy paths. Message and command tags are illustrative.

Notation: `Msg` in **bold**, `Cmd` in `code`, effects italicised.

## Flow A — Pay an invoice (the canonical example)

You forward an email with a PDF invoice to **`pay@kasi.decode.ee`** and get a
reply thread until the invoice is paid.

### 1. Delivery → inbox

Fastmail's catch-all on `kasi.decode.ee` delivers the mail to käsi's account
([04](./04-email.md)). The **inbox-poll subscription** notices it via
`Email/changes`, fetches the raw MIME, and writes an `inbox` row
([03](./03-persistence.md)).

→ emits **`email.received`** `{inbox_id, recipient: "pay@kasi.decode.ee"}`

The runtime logs this message before applying it ([01](./01-architecture.md)).

### 2. Route → new task

The **`email.received`** handler:

- checks the sender against the allow-list ([04](./04-email.md)) — passes;
- finds no matching thread key → this is a **new task**;
- reads the local part `pay` → route `pay` → template `invoice-payment`
  ([04](./04-email.md), [07](./07-skills-and-tools.md)).

→ updates model: new `Task{status: open, route: pay, template: invoice-payment}`
→ returns `[create-workspace, lay-in, provision, spawn-agent-run]`

(The task id needs to be deterministic under replay — derived from the log
offset or supplied via an `id.generated` message ([01](./01-architecture.md)).)

### 3. Prepare the workspace

Effects run concurrently in workers ([01](./01-architecture.md)):

- `create-workspace` — *makes `$WORKDIR/task-$ID/` with `in/` and `out/`*
  ([05](./05-agents-and-tasks.md)).
- `lay-in` — *writes the email text to `in/body.txt` and the PDF part to
  `in/invoice.pdf`* ([02](./02-object-model.md)).
- `provision` — *writes the template's skills into `skills/` and `.mise.toml`,
  runs `mise install`* ([07](./07-skills-and-tools.md)).

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
subscription** sees the process exit.

→ emits **`agent.finished`** `{task_id, run_id, exit: 0, transcript_path}`

The **`agent.finished`** handler returns
`[capture-transcript, harvest-out, assemble-reply]`:

- `capture-transcript` — *copies the session transcript into `archive`*
  (`kind='transcript'`) ([03](./03-persistence.md), [05](./05-agents-and-tasks.md)).
- `harvest-out` — *reads `out/` into MIME parts* ([02](./02-object-model.md)).
- `assemble-reply` — *builds the reply MIME (body = the question, threading
  headers, a completion link), writes a `pending` `outbox` row*
  ([04](./04-email.md)).

→ emits **`email.queued`** → handler sets `Task.status = awaiting-user`.

### 6. Send the reply

The **send** path transmits the `pending` outbox row via JMAP
`EmailSubmission/set` ([04](./04-email.md)); the Fastmail token is resolved from
`secret://fastmail/api-token` at the edge ([06](./06-secrets.md)).

→ emits **`email.sent`** `{outbox_id}` → handler marks the row `sent`.

You receive the agent's question **as a reply in your original thread**.

### 7. You reply with confirmation

You reply "yes, pay it" in the same thread. Steps 1–2 repeat, but now the
**`email.received`** handler matches the thread key → **reply within the existing
task** ([04](./04-email.md), [05](./05-agents-and-tasks.md)):

→ returns `[lay-in (new text into in/), spawn-agent-run (resume session)]`
→ `Task.status = awaiting-agent`

The agent **resumes the same session** ([05](./05-agents-and-tasks.md)), pays the
invoice using its provisioned tool and `secret://route/pay/*` credential
([06](./06-secrets.md), [07](./07-skills-and-tools.md)), and writes a confirmation
plus `out/receipt.pdf`.

→ **`agent.finished`** → `capture-transcript`, `harvest-out`, `assemble-reply`
(body = "Paid. Receipt attached.", attachment = the receipt) → `email.queued` →
`email.sent`.

You get the receipt in the thread.

### 8. You mark it done

You click the **completion link** in the reply ([04](./04-email.md),
[08](./08-web-ui.md)). The web edge validates the token and feeds a message to
the core ([08](./08-web-ui.md)):

→ emits **`task.done`** `{task_id}`

The **`task.done`** handler returns `[archive-task]`:

- `archive-task` — *archives every not-yet-archived `in/` attachment, `out/`
  artifact, and transcript; verifies completeness; then deletes
  `$WORKDIR/task-$ID/`* — archive-then-delete, in that strict order
  ([05](./05-agents-and-tasks.md)).

→ `Task.status = done`. The task lives on entirely in SQLite (emails, artifacts,
transcripts); only the scratch directory is gone.

## Flow B — The clarification loop, abstractly

Flow A steps 5–7 generalise to any multi-turn task:

```
awaiting-agent ──agent.finished (asks)──► assemble-reply ──► awaiting-user
      ▲                                                            │
      └────── email.received (reply in thread) ──► spawn (resume) ─┘
```

Each turn: one **agent run**, one continuous **session**, one exchange in one
**email thread** — the anchoring equivalence ([05](./05-agents-and-tasks.md)),
sustained for as many rounds as the work needs, until **`task.done`**.

## Flow C — Crash and restart (durability)

Suppose käsi is killed right after step 6 emits **`email.sent`** but before you
reply.

On restart ([01](./01-architecture.md), [03](./03-persistence.md)):

1. Load the newest snapshot; replay `message_log` after it with **effects
   suppressed**. Replaying **`email.received`**, **`task.created`**,
   **`agent.finished`**, **`email.queued`**, **`email.sent`** rebuilds the exact
   model: this task in `awaiting-user`. No email is re-sent, no agent re-spawned —
   those were effects, and replay performs none ([01](./01-architecture.md)).
2. Switch to live mode; compute `subscriptions(model)` and start them, including
   the inbox poller.
3. **Reconciliation** ([03](./03-persistence.md)) inspects the model: the outbox
   row is already `sent`, so nothing to resend; no agent is `awaiting-agent`, so
   nothing to resume. The system is exactly where it left off.

Had the crash happened one step earlier — outbox row `pending`, **`email.sent`**
never logged — replay would rebuild the task in a state with a `pending` row, and
the reconciliation subscription would emit `send-email`, delivering the reply
exactly once (the pre-generated `Message-ID` guards against duplicates
([04](./04-email.md))). This is why effects are described-then-interpreted and
outbound sending is a durable, idempotent queue.

## What every flow demonstrates

- **Message → commands → effects → messages**, folded by one reducer
  ([01](./01-architecture.md)).
- **MIME at every boundary** — inbox, `in/`, `out/`, outbox, archive
  ([02](./02-object-model.md)).
- **Secrets resolved only at the edge** ([06](./06-secrets.md)).
- **Durability by logging messages, not effects** ([01](./01-architecture.md),
  [03](./03-persistence.md)).
- **One task ⇔ one thread ⇔ one session**, from first email to completion
  ([05](./05-agents-and-tasks.md)).
