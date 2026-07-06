# Decision 001 — a UI request is a model entry, not a content table

**Status:** accepted (Flow C, stage 3)

## Context

Flow C ([10](./10-flows.md), [05](./05-agents-and-tasks.md)) has an agent raise a
**UI request**: a form spec, an unguessable token, a status, and — once answered —
references to the collected inputs. The docs describe where it lives with some
tension:

- [02](./02-object-model.md): "the request record is a model entry **plus a
  durable row** ([03](./03-persistence.md))".
- [05](./05-agents-and-tasks.md): `mint-ui-request` "writes a `pending`
  `ui_request` row ([03](./03-persistence.md))".
- But [03](./03-persistence.md) defines **no** `ui_request` table — only
  `message_log`, `inbox`, `outbox`, `archive`, `skill`, `secret`. And
  [08](./08-web-ui.md) is explicit that the web edge **reads from the in-RAM
  model**, not from a query layer.

## Decision

The UI request is an **event-sourced model entry only**. Its durable record is
the `register-ui-request` message already written to `message_log`; there is **no
separate `ui_request` content table**. The form spec (a small JSON field list)
rides inside `register-ui-request`. Heavy *answered* content — uploaded files —
goes to the existing `archive` table; secrets go to the secrets database; the
model and log carry only **references** (archive ids, `secret://` URLs).

Adding a `ui_request` table is explicitly rejected.

## Rationale

käsi rebuilds the **entire** model by folding the whole log, with no snapshot
table ([01](./01-architecture.md), [03](./03-persistence.md)). Content tables
exist for *heavy bytes that must not live in the log* (raw MIME, attachments,
transcripts). A request record is **model state** — status, token, a light spec —
so a table would duplicate the log, invite dual-write drift, and buy nothing: the
only reader is the web edge, which reads the model. "Durable row" ([02](./02-object-model.md))
is satisfied by the durable log entry. Fewer moving parts, one source of truth.

## Consequences

- No change to `store/` for the request itself; `content.go` /
  `content_sqlite.go` / `content_memory.go` are untouched by Flow C except the
  already-present `archive` path (`AddArchive`) for file uploads.
- Wording in [02](./02-object-model.md), [03](./03-persistence.md),
  [05](./05-agents-and-tasks.md) is updated: "a model entry, durable via its
  `register-ui-request` log record" — no `ui_request` row.
- If a form spec ever grows heavy (it should not), archive it and carry the id;
  the record stays reference-only.
