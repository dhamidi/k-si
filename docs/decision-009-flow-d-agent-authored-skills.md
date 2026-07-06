# Decision 009 — Flow D: agent-authored skills (Agent Skills directories)

**Status:** accepted (Flow D, stage 3) — supersedes the earlier flat-file draft

## Context

An agent can work out how to do a category of work and write it down for reuse
([07](./07-skills-and-tools.md), [10](./10-flows.md) Flow D). A skill is **not a
single file** — it is an **Agent Skills directory** (the open format käsi's Claude
harness already understands): a folder containing a required `SKILL.md` (YAML
frontmatter with `name` + `description`, then a Markdown body) plus optional
`scripts/`, `references/`, `assets/`, and any other files the skill needs. So Flow
D must carry a **file tree**, not a blob of Markdown. docs/07 frames skills inside
a larger system (templates, tools, cross-task provisioning, UI editing); none of
that exists yet.

## Decision

Build the **authoring slice**, faithful to the Agent Skills format:

- **Authoring.** The agent writes an Agent Skill directory under
  `out/skills/<name>/` — `SKILL.md` plus whatever subtree it needs. `<name>` is the
  folder name and must match the SKILL.md frontmatter `name`. This requires nested
  agent output ([decision-011](./decision-011-nested-agent-output.md)).
- **Detect + store.** `tasks`' `finish-agent-run`, on a successful run, sees a
  `skills/<name>/SKILL.md` in the (now recursive) out-manifest and additively emits
  `store-skill` (a run may author a skill AND reply/raise a request). `store-skill`
  is a **tasks** command/effect: it reads the whole `out/skills/<name>/` tree,
  parses `SKILL.md` frontmatter for `name`/`description`, writes the skill to the
  `skill` table (content = the tree, see
  [decision-010](./decision-010-skills-content-in-a-table-registry-in-the-model.md);
  `origin='agent'`, `origin_task`), provisions it (below), and emits
  `register-skill`.
- **Registry domain.** A new **`skills`** module owns the registry: a content-free
  `Skill` entry (id, name, description, origin, origin_task, version) and the
  `register-skill` handler. Skills are global, not `tasks`-scoped; `store-skill`
  (tasks) → `register-skill` (skills) is the Flow-C mint→register cross-module emit.
- **Provisioning — same task, for now.** `store-skill` writes the tree into the
  workspace as `skills/<name>/…` (a nested `skills/` box beside `in/`/`out/`) — the
  exact layout the harness expects, so the agent finds `./skills/<name>/SKILL.md`.
  Because the workspace persists across turns, the **next turn of the same task**
  has it. The worker prompt documents authoring (`out/skills/<name>/`) and use
  (`./skills/`).

## Out of scope (deferred, named)

Cross-task provisioning from the table at spawn, **task templates** declaring
skills, **tools/mise**, secret namespaces, UI **editing/retiring/promoting**, and
harness-native skill activation config. Cross-task reuse needs the template
registry (absent); until then a stored skill is reusable within its task and
**browsable in the UI** (the added phase) — the honest, shippable increment.

## Consequences

- Depends on [decision-011](./decision-011-nested-agent-output.md) (nested output)
  and [decision-010](./decision-010-skills-content-in-a-table-registry-in-the-model.md)
  (tree stored as a tar blob).
- `tasks`: `store-skill` effect + a `finish-agent-run` branch; a `skilltree` helper
  packs/reads the tree. `email/command_assemble_reply.go` skips the `skills/` tree
  (a skill is never an email attachment).
- New `skills` module wired into the three assembly points; `register-skill` a
  `skills/msg` contract, `store-skill` a tasks-local command.
