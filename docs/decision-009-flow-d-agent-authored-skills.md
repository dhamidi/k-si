# Decision 009 — Flow D: agent-authored skills, and its scope

**Status:** accepted (Flow D, stage 3)

## Context

An agent can work out how to do a category of work and write it down for reuse
([07](./07-skills-and-tools.md), [10](./10-flows.md) Flow D): it leaves
`out/skills/<name>.md`; `finish-agent-run` flags it; `store-skill` writes it to the
`skill` table (`origin='agent'`) and emits `register-skill`, which adds it to the
registry. docs/07 frames skills inside a larger system — task templates that
declare skills+tools, mise-managed tools, cross-task provisioning, UI editing.
None of that infrastructure exists yet (templates are a bare string label; there
is no skill table, registry, or provisioning in code).

## Decision

Build **only the authoring slice** and make it genuinely useful on its own:

- **Path.** `tasks`' `finish-agent-run`, on a successful run, detects a
  `*.skill.md` file in the out-manifest and emits `store-skill` (additive — a run
  may author a skill AND reply/raise a request). `store-skill` is a **tasks**
  command/effect (it has the task's `Work` + `Content`): it reads the file, writes
  the `skill` table (`origin='agent'`, `origin_task`), provisions it (below), and
  emits `register-skill`.
- **Authoring convention — a FLAT filename.** The agent writes
  `out/<name>.skill.md`, not `out/skills/<name>.md`. The harness out-manifest is
  flat (it lists `out/`'s top level, not a recursive tree), so a nested skills
  directory would mean threading nested-output support through both harnesses,
  `writeBox`, and `manifest()` — disproportionate for the MVP and functionally
  identical. `<name>` is the basename minus `.skill.md`. `assemble-reply` skips
  `*.skill.md` (like `request.json`) so a skill never rides out as an attachment.
  (Nested `out/skills/` is a future refinement once nested output is generally
  supported.)
- **Domain.** A new **`skills`** module owns the registry: a `Skill` model entry
  (name, description, origin, origin_task, id, version) and the `register-skill`
  handler that adds it. Skills are cross-task and global — not `tasks`-scoped — so
  they get their own domain, and `store-skill` (tasks) → `register-skill` (skills)
  is the same cross-module emit as Flow C's mint→register.
- **Provisioning — same task only, for now.** `store-skill` copies the skill into
  the run's workspace as `skills/<name>.md` (a new `skills/` box beside `in/`/`out/`
  — a flat sibling directory, so no nested-output problem on this side). Because a
  task's workspace persists across turns until the task is done, the **next turn of
  the same task** finds it with no separate step. The worker prompt documents
  `out/<name>.skill.md` (author) and `./skills/` (use).

## Out of scope (deferred, named so they aren't forgotten)

Cross-task provisioning from the table at spawn, **task templates** declaring
skills, **tools/mise**, **secret namespaces**, UI **editing/retiring/promoting**
skills, and native Claude-harness skill surfacing. Cross-task reuse needs the
template registry, which does not exist; until then a stored skill is reusable
within its task and **visible/reviewable in the UI** (the browsable phase), which
is the honest, shippable increment.

## Consequences

- `tasks`: `store-skill` effect + a `finish-agent-run` branch; the workspace gains
  a `skills/` box to write into.
- New `skills` module (registry model + `register-skill` + exported reads), wired
  into the three assembly points.
- `register-skill` is a `skills/msg` contract; `store-skill` is a tasks-local command.
