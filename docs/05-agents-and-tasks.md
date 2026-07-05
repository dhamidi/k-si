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
- **awaiting-user** — the agent finished and asked for more; a reply has been
  sent and käsi is waiting on the human.
- **done** — the user clicked the completion link ([04](./04-email.md)); archive
  and clean up.

Every transition is a runtime message (`task.created`, `agent.finished`,
`email.sent`, `task.done`) and is therefore logged and replayable.

## Workspace layout

Each task gets a workspace on disk:

```
$WORKDIR/task-$ID/
├── in/            # inputs for the agent (read)
│   ├── body.txt          # the email text (this turn, and prior context)
│   ├── invoice.pdf       # attachments, one file per MIME part
│   └── ...
├── out/           # outputs from the agent (harvested)
│   ├── reply.txt         # becomes the reply body
│   ├── receipt.pdf       # becomes a reply attachment
│   └── ...
├── skills/        # skills provisioned for this template ([07])
├── .mise.toml     # tool versions for this workspace ([07])
└── (harness working files, session/transcript)
```

- **`in/`** is written by käsi before the run: the current email's text and
  attachment parts ([02](./02-object-model.md)), plus enough prior-turn context
  for the agent to continue the conversation.
- **`out/`** is written by the agent and read by käsi after the run. Its text
  becomes the reply body; its other files become attachments
  ([04](./04-email.md)). The contract is simple and file-based: *"put what you
  want to send back into `out/`."*
- Skills and tool pins are provisioned per template ([07](./07-skills-and-tools.md)).

The workspace is **ephemeral** — it is deleted when the task is done. Nothing of
lasting value may live only in the workspace; it must be archived first (below).

## Running an agent through a harness

käsi does **not** implement an agent loop. It shells out to an **official
harness** — the Claude CLI/SDK by default, other harnesses pluggable behind the
same interface.

The `spawn-agent-run` command effect:

1. Ensures the workspace exists and `in/` is populated.
2. Ensures required tools are installed and pinned (`mise install`) and skills
   are present ([07](./07-skills-and-tools.md)).
3. Resolves any `secret://` references the run needs into environment/config at
   the edge ([06](./06-secrets.md)) — plaintext never enters the model or log.
4. Invokes the harness in the workspace, either **starting** a new session (first
   turn of a task) or **resuming** the existing session id (subsequent turns), so
   the conversation is continuous.
5. Registers a subscription that **watches** the harness process and emits
   `agent.finished` (with exit status and the session/transcript location) when
   it exits.

The effect returns immediately after spawning; the long-running work happens in
the worker process, watched by the subscription. The reducer is never blocked by
a running agent ([01](./01-architecture.md)).

Because harnesses differ, käsi defines a thin **harness interface** (start /
resume / locate-transcript / signal) and adapts each official harness to it. The
default adapter targets Claude; adding another is implementing the interface, not
touching the runtime.

## Capturing the transcript

When `agent.finished` fires, käsi **copies the harness's session transcript into
SQLite** (`archive`, `kind='transcript'`, [03](./03-persistence.md)), verbatim.
The transcript is the durable record of what the agent did and thought this
turn; it survives workspace deletion and is the audit trail for the task. It is
stored as-received (typically JSONL) rather than reformatted, so it stays
faithful to the harness.

The transcript is also what lets a later turn *resume* the session: the harness
resumes from its own session store while it exists in the (not-yet-deleted)
workspace; once the task is done and the workspace removed, the archived
transcript remains as the historical record.

## Producing the response

After `agent.finished` (success):

1. **Harvest `out/`** into MIME parts ([02](./02-object-model.md)).
2. If the agent's output indicates the task is **complete or needs the user**,
   emit `assemble-reply` → the reply goes to the outbox and out to the user
   ([04](./04-email.md)), and the task moves to `awaiting-user`.
3. **Archive** this run's artifacts and transcript.

If the agent produced nothing to send (e.g. a purely internal step), no reply is
assembled; the task simply waits for the next message.

## Completion, archival, and cleanup

The user ends a task by clicking the **completion link** in any reply
([04](./04-email.md)), which emits `task.done`. Its handler runs a strict
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

- Agent finishes a turn, asks a question → reply sent → `awaiting-user`.
- User replies in the thread → routed as "reply within task" ([04](./04-email.md))
  → the new text (and any new attachments) is laid into `in/` → a new agent run
  **resumes** the same session → `awaiting-agent`.
- Repeat until the user clicks done.

Each turn is one agent run against one continuous session, one exchange in one
email thread — the anchoring equivalence, sustained across the whole
conversation.
