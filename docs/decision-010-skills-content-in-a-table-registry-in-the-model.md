# Decision 010 — a skill's tree is a tar blob in the `skill` table; registry in the model

**Status:** accepted (Flow D, stage 3) — revised for directory-tree skills

## Context

A skill is an Agent Skills **directory** (decision-009): `SKILL.md` plus optional
`scripts/`, `references/`, `assets/`, and other files. docs/07: "The model holds
the set of available skills; their content lives in SQLite." docs/03 gives the
`skill` table a single `content BLOB`. A skill's payload is now a *file tree*, and
it must survive workspace deletion (the reason skills live in the database) and be
versioned and UNIQUE by name.

## Decision

Store the whole tree, atomically, and split heavy-bytes-from-index exactly as
inbox/archive already are:

- **Content = a tar of the skill directory**, held in the `skill` table's single
  `content BLOB`. A skill is provisioned and versioned as one unit, so one blob per
  skill (per version) is the natural granularity — no per-file table. A small pure
  **`skilltree`** helper packs a `[]mime.Part` (the tree, paths relative to the
  skill root) into a tar and reads it back: `Pack`, `List(tar) []string`,
  `Read(tar, path) []byte`, `Unpack(tar) []mime.Part` — stdlib `archive/tar` only.
- **Table.** `store.Content` gains `AddSkill(SkillRow) (id, error)` (UNIQUE name →
  bump `version`, replace content), `SkillByID`, `SkillByName`, `AllSkills`.
  `SkillRow{ Name, Description, Content /*tar*/, Origin, OriginTask, Version }`.
- **Registry = the `skills` model**, content-free: `Skill{ ID, Name, Description,
  Origin, OriginTask, Version }`, rebuilt from `register-skill` log records. The
  list page reads the registry; a skill's detail page reads its tar from the table
  by id and lists/reads entries with `skilltree`.

`register-skill` carries id + light metadata into the log; the tar never enters the
log or the model. `store-skill` is the only writer, so table and registry can't
drift.

## Rationale

The tree is durable heavy content that must outlive the task — content-table
territory (docs/03), not the log's (references + light metadata only). A tar keeps
the schema exactly as docs/03 (one `content BLOB`), stores the tree atomically
(matching how a skill is provisioned and versioned as a unit), and unpacks with the
standard library — no per-file schema, no ORM. The registry stays content-free so
the model and replay stay cheap. A skill differs from a UI request (decision-001,
model-only) precisely because its payload is a durable file tree.

## Consequences

- New pure `skilltree` package (tar pack/list/read/unpack); `store.Content` skill
  methods + `CREATE TABLE skill` across the interface and both twins.
- The browse UI lists a skill's files from its tar and shows any one file's text.
- Versioning is in the schema; edit/retire deferred (decision-009), so v1 is
  written and provisioned as latest.
