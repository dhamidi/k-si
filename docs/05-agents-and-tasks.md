# 05 — Agents & tasks

A **task** is the central business object: one task = one email thread = one
agent session. This document defines the task lifecycle, the workspace layout,
how käsi runs agents through official harnesses, how transcripts are captured,
and how a finished task is archived and cleaned up.

## The equivalence that anchors everything

> **one task ⇔ one email thread ⇔ one agent session**

- **One email thread.** A task begins with an email to a route and continues as
  a normal reply thread ([04](./04-email.md)). Every user reply and every agent
  response is a message in that one thread.
- **One agent session.** A task corresponds to a single, resumable agent
  conversation. Each turn is an **agent run** (a harness invocation), and the
  runs share one continuous session/transcript so the agent keeps its context
  across turns.

This is why routing distinguishes "reply within a task" from "new task"
([04](./04-email.md)): a reply feeds the *same* session; a fresh email starts a
*new* one.

## The main agent and worker agent runs

käsi in its orchestrator role is the **main agent**: it owns the global email
inbox/outbox and manages the in/out boxes of every **agent run** it spawns. Each
task's actual work is done by a **worker agent run** — a harness (Claude by
default) executing inside the task's workspace. The main agent never does the
task's reasoning itself; it prepares inputs, runs the harness, and harvests
outputs. "Manages the inboxes and outboxes of other agent runs" means exactly
this: laying mail into `in/`, harvesting `out/`, and moving results between the
worker and the outside world.

The orchestrator is code, and runs silently. Its **conversational face** is the
*supervisor*: when you want to drive käsi itself — list, inspect, stop, resume, or
archive tasks and requests — you talk to the supervisor, an agent with tools over
the data model ([11](./11-supervisor.md)).

## Task lifecycle

States (held in the model, [01](./01-architecture.md)):

```
            new email (routed)
                  │
                  ▼
     ┌───────►  open  ──────────────┐
     │            │                 │
 user replies     │ spawn run       │
     │            ▼                 │
awaiting-user ◄─ awaiting-agent     │
     ▲            │ run finishes     │
     │            ▼                 │
     └──── reply sent (needs more)  │
                  │                 │
             user clicks "done" ────┘
                  ▼
                done  ──►  archived, workspace removed
```

- **open** — task created from an inbound email; template selected.
- **awaiting-agent** — a worker agent run is (or should be) executing.
- **awaiting-user** — the agent finished and asked for more (or was **stopped**,
  below); käsi is waiting on the human. When a reply was sent, it's waiting on
  that; when stopped, it's waiting for your next instruction.
- **done** — a participant clicked the completion link ([04](./04-email.md)), or
  the supervisor archived it ([11](./11-supervisor.md)); archive and clean up.

A run can also be **stopped** mid-flight (from the web UI or the supervisor) — see
*Stopping a run* below; a stop lands the task back in `awaiting-user`.

Every transition is an imperative runtime message (`route-email`,
`finish-agent-run`, `mark-email-sent`, `finish-task`) and is therefore logged and
replayable ([01](./01-architecture.md)).

## Workspace layout

Each task gets a workspace on disk:

```
$WORKDIR/task-$ID/
├── in/                   # inputs for the agent (read)
│   ├── body.txt          # the email text (this turn, and prior context)
│   ├── invoice.pdf       # attachments, one file per MIME part
│   └── ...
├── out/                  # outputs from the agent (harvested) — a file TREE
│   ├── reply.txt         # becomes the reply body
│   ├── receipt.pdf       # becomes a reply attachment
│   └── skills/pay/SKILL.md   # a skill the agent authored, nested (decision-011)
├── .claude/skills/       # skills provisioned for this run ([07], decision-009)
│   └── <name>/SKILL.md   #   where the Claude CLI discovers project skills
├── store -> $STATE/store # the agent's persistent data store (Flow F, decision-012)
├── .mise.toml            # tool versions for this workspace ([07])
└── (harness working files, session/transcript)
```

- **`in/`** is written by käsi before the run: the current email's text and
  attachment parts ([02](./02-object-model.md)), plus enough prior-turn context
  for the agent to continue the conversation.
- **`out/`** is written by the agent and read by käsi after the run. It is a
  file **tree**, not a flat directory: the agent may write nested paths (e.g.
  `out/skills/<name>/SKILL.md`) and käsi harvests them by relative path
  ([decision-011](./decision-011-nested-agent-output.md)). Its `reply.txt`
  becomes the reply body; its other files become attachments
  ([04](./04-email.md)). The contract is simple and file-based: *"put what you
  want to send back into `out/`."*
- **`.claude/skills/`** is where skills are provisioned for the run — the
  location the Claude CLI natively discovers project skills, relative to its
  cwd (the task dir), so a run finds `./.claude/skills/<name>/SKILL.md`
  ([07](./07-skills-and-tools.md),
  [decision-009](./decision-009-flow-d-agent-authored-skills.md)). Skills and
  tool pins are provisioned per template.
- **`store`** is a symlink to the agent's persistent data store,
  `$STATE/store`, linked in at spawn (Flow F,
  [decision-012](./decision-012-the-agent-store-is-an-edge-outside-the-log.md)).
  It holds the agent's live working data — SQLite DBs, scratch scripts — that
  must survive the workspace's deletion. Unlike everything else here it lives
  **outside** the ephemeral workspace: the link makes it reachable during a run,
  but its contents are neither harvested nor archived ([03](./03-persistence.md)).

The workspace is **ephemeral** — it is deleted when the task is done. Nothing of
lasting value may live only in the workspace; it must be archived first (below).

## Running an agent through a harness

käsi does **not** implement an agent loop. It shells out to an **official
harness** — the Claude CLI/SDK by default, other harnesses pluggable behind the
same interface.

The `spawn-agent-run` command effect:

1. Ensures the workspace exists and `in/` is populated.
2. Ensures required tools are installed and pinned, and skills present, from the
   task's template plus any skills authored by earlier runs
   ([07](./07-skills-and-tools.md)). Tool installs go to a **shared, persistent
   mise data directory** so they carry across tasks and are not re-downloaded,
   and käsi **pre-trusts** each workspace's `.mise.toml` so the agent never has to
   run `mise trust` or deal with a trust prompt (see [07](./07-skills-and-tools.md)).
3. Resolves any `secret://` references the run needs into environment/config at
   the edge ([06](./06-secrets.md)) — plaintext never enters the model or log.
4. Invokes the harness in the workspace, either **starting** a new session (first
   turn of a task) or **resuming** the existing session id (subsequent turns), so
   the conversation is continuous.
5. Registers a subscription that **watches** the harness process and emits
   `finish-agent-run` when it exits. That message is **complete**: it carries the
   exit status, the transcript location, and a manifest of what the run left in
   `out/` (and any authored skill), so the handler decides what to do next purely
   from the message ([01](./01-architecture.md)).

The effect returns immediately after spawning; the long-running work happens in
the worker process, watched by the subscription. The reducer is never blocked by
a running agent ([01](./01-architecture.md)).

Because harnesses differ, käsi defines a thin **harness interface** (start /
resume / locate-transcript / signal) and adapts each official harness to it. The
default adapter targets Claude; adding another is implementing the interface, not
touching the runtime.

## Capturing the transcript

When `finish-agent-run` fires, käsi **copies the harness's session transcript
into SQLite** (`archive`, `kind='transcript'`, [03](./03-persistence.md)),
verbatim.
The transcript is the durable record of what the agent did and thought this
turn; it survives workspace deletion and is the audit trail for the task. It is
stored as-received (typically JSONL) rather than reformatted, so it stays
faithful to the harness.

The transcript is also what lets a later turn *resume* the session: the harness
resumes from its own session store while it exists in the (not-yet-deleted)
workspace; once the task is done and the workspace removed, the archived
transcript remains as the historical record.

**While a run is in flight**, the transcript exists only in the workspace, being
written by the harness. That in-progress stream is exactly what the web UI reads
to show a *live* transcript ([08](./08-web-ui.md)); the SQLite archive appears
once `finish-agent-run` fires. So there are two read sources — the workspace for a
running run, the archive for a finished one — and the UI picks by run state.

## Stopping a run

If you catch an agent going off track, you can **stop** it — from the web UI
([08](./08-web-ui.md)) or by asking the supervisor ([11](./11-supervisor.md)).
Either way it is a message ([01](./01-architecture.md)):

1. A `stop-agent-run` message (complete: task id + run id) is emitted. Its handler
   marks the run as stopping and returns `[signal-agent-run]`.
2. `signal-agent-run` — *signals the harness process to terminate* (graceful
   first, then hard).
3. The **agent-watch subscription** sees the process exit and emits
   `finish-agent-run` flagged **stopped** — the same message as a normal exit, so
   the same machinery runs: the **transcript so far is captured**
   ([03](./03-persistence.md)).
4. Because the run was stopped, the handler **assembles no reply** and leaves the
   task in `awaiting-user`. You decide what's next: reply in the thread (or ask
   the supervisor) to **resume** the session with a correction, or mark it done /
   archive it.

Stopping is thus safe and lossless: it halts the process, keeps everything the
agent had produced, and hands control back to you. Resuming afterwards continues
the *same* session ([05](./05-agents-and-tasks.md) — *Multi-turn conversations*),
now steered by your correction.

## Producing the response

After `finish-agent-run` (success):

1. **Harvest `out/`** into MIME parts ([02](./02-object-model.md)).
2. If the agent's output indicates the task is **complete or needs the user**,
   emit `assemble-reply` → the reply goes to the outbox and out to the user
   ([04](./04-email.md)), and the task moves to `awaiting-user`.
3. **Archive** this run's artifacts and transcript.
4. **Store any authored skill** (below).

If the agent produced nothing to send (e.g. a purely internal step), no reply is
assembled; the task simply waits for the next message.

## Agents that author skills

A run may **produce a skill** as part of its work — for example, working out how
to reconcile a particular vendor's invoices and writing that know-how down for
next time. The workflow is edge-does-I/O, model-stays-pure:

1. The agent writes the skill as an **Agent Skills directory** under
   `out/skills/<name>/` — a `SKILL.md` (YAML frontmatter `name`+`description`, then
   Markdown) plus any `scripts/`/`references/` files
   ([decision-009](./decision-009-flow-d-agent-authored-skills.md)). The file-based
   contract mirrors the reply contract: *"leave a skill in `out/skills/` to teach
   käsi."*
2. The `finish-agent-run` manifest flags it. The handler emits a `store-skill`
   command.
3. The `store-skill` effect writes the skill's content to the `skill` table in
   SQLite ([03](./03-persistence.md)) and emits `register-skill` — a complete
   message naming the skill and referencing its stored content.
4. The `register-skill` handler adds it to the in-RAM skill registry
   ([07](./07-skills-and-tools.md)).

From then on the skill is available to **subsequent agent runs**: the very next
turn of this task, and any other task whose provisioning includes it
([07](./07-skills-and-tools.md)). Because the skill lives in SQLite, it survives
this task's workspace deletion — the whole point of storing it durably rather
than leaving it in the ephemeral `out/`.

## Agents that request input via the web

Sometimes email is the wrong channel for the agent's ask: it needs a **file**, a
set of **structured fields**, or a **secret** the user shouldn't paste into an
email. For these the agent raises a **UI request** ([02](./02-object-model.md),
[08](./08-web-ui.md)) — it preps the request, käsi turns it into a link, the user
taps the link and answers on a web page, and the answer flows back into the task.

The agent chooses the channel: a plain question goes in `out/reply.txt` and is
answered by email reply ([04](./04-email.md)); a request needing files, structure,
or secrets goes as a UI request. Both can coexist — the reply body can explain
*why* while the link collects the input.

The loop, edge-does-I/O and model-stays-pure:

1. **Prep.** The agent writes a form spec to `out/request.json` (fields, each with
   a type — `text` / `longtext` / `choice` / `file` / `secret` — and a required
   flag), alongside an optional `out/reply.txt` explaining the ask. The file-based
   contract mirrors the reply and skill contracts.
2. **Mint.** The `finish-agent-run` manifest flags the request; the handler emits
   a `mint-ui-request` command. Its effect generates an unguessable **token**,
   builds the capability link (keyed by the raising run's id,
   [decision-003](./decision-003-request-links-mirror-the-completion-link-keyed-by-run-id.md)),
   and emits `register-ui-request` — a complete message carrying the run id,
   token, form spec, and link. The request becomes a `pending` **model entry**,
   durable via that log record, not a separate table
   ([decision-001](./decision-001-ui-request-is-a-model-entry-not-a-content-table.md)).
3. **Deliver.** `register-ui-request` adds the request to the model, sets the task
   to `awaiting-user`, and drives `assemble-reply` so the email reply contains the
   agent's message **and the link** ([04](./04-email.md)). The user gets a normal
   email; tapping the link opens the form.
4. **Answer.** On the web ([08](./08-web-ui.md)) the user fills fields, attaches
   files, and provides secrets. The web edge does all the I/O: it stores uploads
   in `archive`, writes secret fields to the secrets database (getting `secret://`
   URLs back), and emits `answer-ui-request` — a complete message carrying only
   **references** (file archive ids, `secret://` URLs), never plaintext
   ([06](./06-secrets.md)).
5. **Resume.** The `answer-ui-request` handler marks the request `answered` and
   returns `[lay-in-answers, spawn-agent-run]`. `lay-in-answers` writes the text
   answers and uploaded files into `in/`; `spawn-agent-run` resolves any
   `secret://` references into the harness environment at the edge
   ([06](./06-secrets.md)) and **resumes the same session**. The task returns to
   `awaiting-agent` and the agent continues with what it asked for.

This is faster than an email round-trip for anything structured, and it keeps
secrets out of email and out of the log entirely — the whole point of the
mechanism.

## Completion, archival, and cleanup

The user ends a task by clicking the **completion link** in any reply
([04](./04-email.md)), which emits `finish-task`. Its handler runs a strict
**archive-then-delete** sequence:

1. **Archive everything not yet archived**: inbound attachments in `in/`, any
   remaining `out/` artifacts, and all run transcripts, into the `archive` table
   ([03](./03-persistence.md)).
2. **Verify** the archive is complete (every workspace file accounted for by a
   stored, hash-matching row).
3. **Only then delete** `$WORKDIR/task-$ID`.
4. Mark the task `done` in the model.

The ordering is an invariant: **never delete a workspace before its contents are
provably archived.** After cleanup the task still fully exists — its emails
(inbox/outbox), artifacts, and transcripts are all in SQLite — only the scratch
directory is gone.

## Multi-turn conversations

The clarification loop ([10](./10-flows.md)) falls straight out of the model:

- Agent finishes a turn, asks a question → reply sent to the task's participants
  → `awaiting-user`.
- A participant replies in the thread → routed as "reply within task"
  ([04](./04-email.md)) → the new text (and any new attachments) is laid into
  `in/` → a new agent run **resumes** the same session → `awaiting-agent`.
- Repeat until a participant clicks done.

Each turn is one agent run against one continuous session, one exchange in one
email thread — the anchoring equivalence, sustained across the whole
conversation.
