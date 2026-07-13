# 07 — Skills & tools

An agent run is only as capable as what the workspace gives it. käsi provisions
two kinds of capability into a workspace before running the harness:

- **Skills** — reusable instruction/prompt bundles (know-how).
- **Tools** — CLI programs, installed and pinned via **mise** (executables).

Both are declared by a **task template** and materialised into the workspace at
spawn time ([05](./05-agents-and-tasks.md)).

## Skills

A **skill** is a named, reusable bundle of instructions that teaches an agent how
to do a category of work — e.g. an `invoice-payment` skill that explains how to
read an invoice, confirm the amount, and execute a payment safely. Skills are the
reusable "how" that keeps task templates small and consistent.

- **Representation.** Content (Markdown instructions plus lightweight metadata:
  name, description, which tools it expects). It can travel as a MIME part
  ([02](./02-object-model.md)) but is primarily a registry entry in the model
  plus content stored durably in the `skill` table ([03](./03-persistence.md)).
- **Registry.** The model holds the set of available skills; their content lives
  in SQLite. Skills are edited from the web UI ([08](./08-web-ui.md)).
- **Provisioning.** When a run is spawned, every registered skill is written into
  the workspace under `.claude/skills/<name>/` — where the Claude CLI discovers
  project skills, relative to its cwd (the task dir) — so the agent finds and uses
  them natively (decision-009). Per-route *selection* of skills is what task
  templates will add; until then all skills are provisioned to every run, which the
  Agent Skills progressive-disclosure model (name+description first, body on demand)
  makes correct rather than noisy.
- **Composability.** A template references skills by name; several templates can
  share one skill. Improving a skill improves every route that uses it. *(Templates
  are not yet built — see [decision-009](./decision-009-flow-d-agent-authored-skills.md);
  today every run gets every skill.)*

Skills mirror the "skills" concept the Claude harness already understands, so the
default adapter can surface them natively; other harness adapters map them to
their own mechanism ([05](./05-agents-and-tasks.md)).

### Skills authored by agents

Skills are not only human-authored. **An agent can write a skill during a task**,
and käsi keeps it for future runs. When a run leaves a skill in `out/skills/`, the
`finish-agent-run` handler emits a `store-skill` command; the effect writes the
skill to the `skill` table (`origin='agent'`) and emits `register-skill`, which
adds it to the registry ([05](./05-agents-and-tasks.md),
[03](./03-persistence.md)). Two consequences:

- **It persists in SQLite**, so it outlives the task's ephemeral workspace — the
  reason we store skills in the database rather than on disk.
- **It is available to subsequent agent runs**: immediately to the next turn of
  the same task, and to other tasks once they reference it (or once it is promoted
  from the UI). This is how käsi accumulates know-how over time instead of
  re-deriving it each task.

The write path is the same registry either way; only `origin` differs. Retiring a
skill — agent-authored or UI-authored — is done from the UI ([08](./08-web-ui.md))
with a per-skill **Remove** control; see [Removing a skill](./feature-skill-removal.md)
for what that clears (the registry entry and the stored tree).

## Tools via mise

A **tool** is a CLI program an agent may invoke — `jq`, a PDF text extractor, a
payment provider's CLI, `git`, etc. käsi manages tools with
[mise](https://mise.jdx.dev/), a version manager and task runner, rather than
assuming whatever happens to be on the host.

Why mise:

- **Pinned, reproducible versions.** A template declares exact tool versions, so
  a run behaves the same today and next month. This matters for replayable,
  auditable behaviour.
- **Per-workspace declaration, shared installs.** Each workspace carries its own
  `.mise.toml` so tool *sets* don't collide between task types — but the actual
  installs live in a **shared, persistent mise data directory** (below), so tools
  are downloaded once and reused.
- **Declarative install.** `mise install` in the workspace makes the declared
  tools present on `PATH` for the harness; no bespoke installer code.
- **One mechanism.** Tooling is "add a line to a template," not shell scripting
  scattered through the codebase.

### Persistence and trust — so the agent never has to think about mise

Two operational rules make mise invisible to the agent:

- **Installs persist across tasks.** käsi points mise at a **shared data
  directory** (via `MISE_DATA_DIR`) that lives *outside* any workspace and is not
  deleted with a task. A tool installed for one task is already present for the
  next; `mise install` becomes a fast no-op once a version is cached. Tool
  binaries are never re-downloaded per task, and never lost when a workspace is
  cleaned up ([05](./05-agents-and-tasks.md)).
- **Workspaces are pre-trusted.** mise refuses to load a config it doesn't trust.
  käsi runs `mise trust` on each workspace's `.mise.toml` as part of
  `provision-workspace`, *before* the harness starts. The agent therefore finds
  its tools already on `PATH` and never has to run `mise trust`, approve a prompt,
  or figure out the tooling — it just uses the tools.

### How tools are declared and provisioned

- A **task template** lists the tools (and versions) its work needs.
- At spawn ([05](./05-agents-and-tasks.md)), `provision-workspace` writes the
  workspace `.mise.toml` from the template, `mise trust`s it, and runs
  `mise install` against the shared data dir (idempotent; cached across tasks),
  then the harness is invoked with mise-managed tools on `PATH`.
- Tools that need credentials get them at the edge via `secret://` resolution
  ([06](./06-secrets.md)) — injected into the environment, never written to disk
  in plaintext.

### Registry

The model holds a **tool registry** (name → mise spec/version) so templates
reference tools by name and the concrete version is resolved centrally. Editing
the registry or a template is done from the web UI ([08](./08-web-ui.md)).

## Task templates tie it together

A **task template** is the unit a route selects ([04](./04-email.md)). It bundles:

- the **prompt / role** for this category of work,
- the **skills** to provision,
- the **tools** (mise specs) to install,
- any **secret namespaces** the work may draw on ([06](./06-secrets.md)).

```
route "pay"  ->  template "invoice-payment"
                   prompt:  "You pay invoices safely…"
                   skills:  [invoice-payment, careful-money]
                   tools:   [pdftotext@…, stripe-cli@…]
                   secrets: route/pay/*
```

Templates are configuration in the model, editable from the UI. This is how a new
capability is added end to end: define a template (prompt + skills + tools), bind
a local part to it ([04](./04-email.md)), and the address is live — no code
change, no mail-provider change.
