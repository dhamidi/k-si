# Decision 010 — skill content in the `skill` table, registry in the model

**Status:** accepted (Flow D, stage 3)

## Context

A skill is Markdown instructions + light metadata (name, description, origin).
docs/07: "The model holds the set of available skills; their content lives in
SQLite." docs/03 defines a `skill` table. This is unlike a UI request (decision-001,
a pure model entry) because a skill's content is a durable *blob* that outlives
every task — the reason it goes to the database rather than a workspace file — and
because a skill is UNIQUE by name and versioned.

## Decision

Split it exactly as inbox/archive already are — **heavy bytes in a content table,
the index in the model**:

- The **`skill` table** (in the content store, main DB — where inbox/outbox/archive
  live) holds the durable content: `AddSkill(SkillRow) (id, error)` and
  `SkillByID(id) (SkillRow, error)`, plus a lookup by name for the UNIQUE
  constraint. `SkillRow{Name, Description, Content, Origin, OriginTask, Version}`.
- The **registry** is the `skills` model: one `Skill{ID, Name, Description, Origin,
  OriginTask, Version}` entry per skill (NO content — that stays in the table),
  rebuilt from `register-skill` log records. The web list reads the registry from
  the model; a skill's detail page reads its content from the table by id.

`register-skill` carries the id + metadata (light) into the log; the content never
enters the log or the model. `store-skill` is the only writer of the table and the
emitter of `register-skill`, so table and registry can't drift.

## Rationale

The content is a blob that must survive workspace deletion (the whole point of a
skill, docs/07) — that is exactly what content tables are for (docs/03), and what
the log is NOT for (references and light metadata only, docs/03). Keeping the
registry content-free keeps the model small and the replay cheap, and matches the
inbox pattern (raw MIME in the table, a reference in the model). A skill differs
from a UI request (decision-001, model-only) precisely because its payload is
durable heavy content, not a transient field set.

## Consequences

- `store.Content` gains `AddSkill`/`SkillByID`/`SkillByName` across the interface +
  both twins; the SQLite twin adds the `CREATE TABLE skill` from docs/03.
- The `skills` model holds content-free registry entries.
- Versioning (`version` bumped on edit) is in the schema now; edit/retire is
  deferred (decision-009), so v1 is written and used as latest.
