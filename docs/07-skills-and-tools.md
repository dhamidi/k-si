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
  plus stored content.
- **Registry.** The model holds the set of available skills; their content is
  stored durably. Skills are edited from the web UI ([08](./08-web-ui.md)).
- **Provisioning.** When a run is spawned, the skills named by its template are
  written into the workspace (e.g. `task-$ID/skills/`) in the layout the harness
  expects, so the agent discovers and uses them.
- **Composability.** A template references skills by name; several templates can
  share one skill. Improving a skill improves every route that uses it.

Skills mirror the "skills" concept the Claude harness already understands, so the
default adapter can surface them natively; other harness adapters map them to
their own mechanism ([05](./05-agents-and-tasks.md)).

## Tools via mise

A **tool** is a CLI program an agent may invoke — `jq`, a PDF text extractor, a
payment provider's CLI, `git`, etc. käsi manages tools with
[mise](https://mise.jdx.dev/), a version manager and task runner, rather than
assuming whatever happens to be on the host.

Why mise:

- **Pinned, reproducible versions.** A template declares exact tool versions, so
  a run behaves the same today and next month. This matters for replayable,
  auditable behaviour.
- **Per-workspace isolation.** Each workspace carries its own `.mise.toml`; tool
  sets don't collide between task types.
- **Declarative install.** `mise install` in the workspace makes the declared
  tools present on `PATH` for the harness; no bespoke installer code.
- **One mechanism.** Tooling is "add a line to a template," not shell scripting
  scattered through the codebase.

### How tools are declared and provisioned

- A **task template** lists the tools (and versions) its work needs.
- At spawn ([05](./05-agents-and-tasks.md)), käsi writes the workspace
  `.mise.toml` from the template and runs `mise install` (idempotent; cached
  across runs), then invokes the harness with mise-managed tools on `PATH`.
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
