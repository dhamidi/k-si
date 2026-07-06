# Decision 011 — agent output is a file tree (nested `out/`)

**Status:** accepted (Flow D, stage 3)

## Context

Until now an agent's `out/` was flat: `reply.txt`, `request.json`, an attachment
or two — and the harness out-manifest listed only `out/`'s top level, while the
workspace's `writeBox` wrote each part at `filepath.Base(name)`, discarding any
path. Agent Skills (decision-009) are **directory trees** (`skills/<name>/SKILL.md`,
`skills/<name>/scripts/…`, `references/…`), so an agent must be able to write a
nested subtree into `out/`, and käsi must see and carry the whole tree. Attachments
with subdirectories are the same need. So `out/` becomes a **file tree**.

## Decision

Make agent output a tree end to end, as a reusable capability (not skills-specific):

- **Manifest is recursive.** Every `Harness` lists `out/` as relative paths for the
  full tree — `reply.txt`, `skills/pay-invoice/SKILL.md`,
  `skills/pay-invoice/scripts/extract.py`. The Claude twin walks `out/`; the sim
  twin's scripted turn already names full paths.
- **Workspace boxes preserve paths.** `writeBox` (and thus `WriteOut`, `LayIn`,
  `WriteSkills`) writes each part at its **relative path** under the box, creating
  intermediate directories — no longer `filepath.Base`. `Harvest`/`Files` already
  read the tree; they keep the relative path. Paths are validated to stay within
  the box (no `..`, no absolute) — an agent must not escape its workspace.
- **The sim `agent { out <path> … }` vocab** accepts a nested `<path>`
  (`out skills/x/SKILL.md "…"`), so ring-1/2 scenarios exercise trees offline.
- **Control files stay top-level.** `reply.txt` and `request.json` are read from
  `out/`'s root only. `assemble-reply` turns non-control top-level files into
  attachments but **excludes the `skills/` subtree** (a skill is stored/provisioned,
  never emailed) — alongside the existing `reply.txt`/`request.json` skips.

## Rationale

A recursive manifest + path-preserving boxes is the minimal, general way to let an
agent emit structured output; skills are the first user but attachments-with-layout
and future authored artifacts benefit too. Keeping control files top-level keeps
the reply/request detection unambiguous and cheap. Path validation on write is the
one security-relevant addition — the workspace is a sandbox boundary.

## Consequences

- `agents/harness_claude.go` `manifest()` walks the tree; `workspace/*` `writeBox`
  preserves relative paths + validates them; the sim harness and the `agent` vocab
  handle nested paths.
- `email/command_assemble_reply.go` skips the `skills/` subtree.
- Existing flat scenarios and cassettes are unaffected (a flat name is a
  depth-1 relative path). Regression: a scenario writing a nested `out/` file and
  asserting the manifest + workspace carry it, and that it does not ride the reply.
