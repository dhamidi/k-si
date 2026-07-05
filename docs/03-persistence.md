# 03 — Persistence

All durable state lives in SQLite (via `mattn/go-sqlite3`). There are **two
databases**, kept separate on purpose:

- the **main database** — the message log, the inbox/outbox MIME, and archives;
- the **secrets database** — credentials, addressed by `secret://` URLs
  ([06](./06-secrets.md)).

This document defines what lives where and the invariants each store must hold.
The schemas below are illustrative of intent, not a migration script.

## Two roles: derivation vs. content

Recall the split from [00](./00-vision.md), Principle 2:

- The **message log** is the source of truth for *state*. The model is a pure
  fold of the log ([01](./01-architecture.md)).
- The **content tables** (inbox, outbox, archives, secrets) are the source of
  truth for *bytes* the model refers to by id.

The model itself is never persisted at all — there are no snapshots
([01](./01-architecture.md)). On a fresh start: replay the whole log to rebuild
the model, which points into the content tables for the heavy data.

Keep these roles clean. The log holds small runtime messages that *reference*
content by id; it does not inline PDFs or transcripts. Content tables hold bytes
but no business logic.

## Main database

### `message_log` — the event source

Append-only. The heart of persistence.

```sql
CREATE TABLE message_log (
  id         INTEGER PRIMARY KEY,          -- monotonic, = replay order
  tag        TEXT    NOT NULL,             -- imperative, e.g. "route-email"
  payload    BLOB    NOT NULL,             -- JSON; references, never secrets
  cause_id   INTEGER,                      -- message that caused this one (nullable)
  created_at TEXT    NOT NULL              -- RFC3339; recorded, used on replay
);
```

Rules:

- **Append-only.** Rows are never updated or deleted in normal operation. (The
  only sanctioned pruning is dropping the message stream of long-`done` tasks if
  the log ever grows unwieldy — see [01](./01-architecture.md).)
- **Write before apply.** A message is committed to the log *before* the reducer
  applies it, so a crash can never lose an applied message.
- **No secrets, no bulk content.** Payloads carry ids and `secret://` URLs, not
  plaintext credentials or file bytes.
- **Ordered by `id`.** Replay is `SELECT ... ORDER BY id`. The id is the logical
  clock.

There is **no snapshot table.** The model is rebuilt by folding the whole log on
every startup ([01](./01-architecture.md)); no derived state is persisted, so
there is nothing to fall out of sync.

### `inbox` — inbound MIME

```sql
CREATE TABLE inbox (
  id          INTEGER PRIMARY KEY,
  message_id  TEXT    NOT NULL,   -- RFC 5322 Message-ID
  raw         BLOB    NOT NULL,   -- original bytes, as received
  parsed      BLOB,               -- parsed/normalised representation (optional cache)
  recipient   TEXT    NOT NULL,   -- envelope recipient, drives routing
  received_at TEXT    NOT NULL,
  status      TEXT    NOT NULL    -- 'new' | 'routed' | 'ignored'
);
```

Fastmail delivery lands raw MIME here first (see [04](./04-email.md)); *then* a
`route-email` runtime message carries the routing facts (recipient, sender,
threading keys) inline and references the row by id. The message is complete —
the routing handler never re-opens this row ([01](./01-architecture.md)). Storing
raw bytes means we can re-parse with improved code later and never lose fidelity.

### `outbox` — outbound MIME

```sql
CREATE TABLE outbox (
  id          INTEGER PRIMARY KEY,
  task_id     INTEGER NOT NULL,
  message_id  TEXT    NOT NULL,   -- our generated Message-ID
  in_reply_to TEXT,               -- threads the reply
  raw         BLOB    NOT NULL,   -- assembled MIME, ready to send
  status      TEXT    NOT NULL,   -- 'pending' | 'sent' | 'failed'
  created_at  TEXT    NOT NULL,
  sent_at     TEXT
);
```

The outbox is a durable send queue. A handler's command writes a `pending` row;
a send effect transmits it via JMAP and emits `mark-email-sent`, whose handler
marks it `sent`. This makes sending **crash-safe and idempotent**: after a
restart a reconciliation subscription re-emits `send-email` for every
still-`pending` row (see *Reconciliation* below). The `Message-ID` is generated
before send so a duplicate send is detectable.

### `archive` — files, artifacts, transcripts

When a task finishes, its workspace is deleted ([05](./05-agents-and-tasks.md)),
so everything worth keeping is archived first.

```sql
CREATE TABLE archive (
  id          INTEGER PRIMARY KEY,
  task_id     INTEGER NOT NULL,
  kind        TEXT    NOT NULL,   -- 'attachment' | 'artifact' | 'transcript'
  agent_run   INTEGER,            -- for transcripts/artifacts
  filename    TEXT,
  content_type TEXT,
  sha256      TEXT    NOT NULL,   -- content hash; enables dedup
  bytes       BLOB    NOT NULL,
  created_at  TEXT    NOT NULL
);
```

- **Attachments** — inbound parts laid into `in/`.
- **Artifacts** — files the agent left in `out/`.
- **Transcripts** — the harness session transcript, copied verbatim
  ([05](./05-agents-and-tasks.md)). Stored as-is (typically JSONL) under
  `kind='transcript'`.

Content-hashing (`sha256`) lets identical bytes be stored once and referenced
many times.

### `skill` — reusable skills, including agent-authored ones

Skills are durable and outlive any single task, so they get their own table
rather than living in the per-task `archive` (which is deleted on task
completion). This table backs the skill registry ([07](./07-skills-and-tools.md))
and is where a skill an **agent writes during a task** is stored so it is
available to later agent runs.

```sql
CREATE TABLE skill (
  id          INTEGER PRIMARY KEY,
  name        TEXT    NOT NULL UNIQUE,  -- referenced by task templates
  description TEXT,
  content     BLOB    NOT NULL,         -- the instruction bundle (Markdown + meta)
  origin      TEXT    NOT NULL,         -- 'ui' | 'agent'
  origin_task INTEGER,                  -- task that authored it, if origin='agent'
  version     INTEGER NOT NULL,         -- bumped on edit; provisioning uses latest
  updated_at  TEXT    NOT NULL
);
```

An agent authoring a skill (see [07](./07-skills-and-tools.md)) results in a row
here with `origin='agent'`; the model's skill registry gains a lightweight entry
referencing it; and every subsequent `provision-workspace` can lay the skill's
`content` into a workspace. Storing skills durably in SQLite — separate from the
ephemeral workspace — is exactly what lets a skill created in one run survive
into the next.

## Secrets database (separate file)

Credentials live in their **own** SQLite database file, never mixed with the
message log or content tables. This physical separation means the log — which is
replayed and inspected freely — never contains plaintext secrets, and the
secrets file can be given tighter permissions and a separate backup policy. Its
schema and the `secret://` resolution model are in [06](./06-secrets.md).

## Reconciliation, not effect-replay

Replay rebuilds *state* with effects suppressed ([01](./01-architecture.md)). It
does **not** re-perform pending effects. Anything that must still happen after a
crash is driven by the model in live mode, via subscriptions that reconcile
declared intent against reality:

- `outbox.status = 'pending'` → subscription emits `send-email` until it's
  `sent`.
- a task in `awaiting-agent` with no live agent-run process → emit
  `spawn-agent-run` to resume it.

This is why sends and spawns are idempotent and keyed by durable ids: the
reconciler may fire again after a restart, and re-firing must be safe.

## Why SQLite, and why in-RAM objects

- **One file, no server.** Fits the single-process, single-machine design and
  the dependency-light goal. Backups are file copies.
- **Transactions.** "Append to log and update model" is protected; "write outbox
  row" is atomic.
- **Bytes and queues in one place.** MIME blobs, the log, and the send queue all
  live in the same engine without extra infrastructure.

Business objects stay in RAM ([01](./01-architecture.md)) because they are small,
frequently traversed, and cheaply rebuilt from the log. SQLite stores what is
large (blobs), what is a durable boundary (inbox/outbox), and what must be
replayable (the log) — not the live object graph.
