---
name: use-kit
description: "Use the Kit provider-driven code generator in a project: discover providers/components, write and apply manifests, generate components, author providers for your own component families, inspect the runtime via the REPL, and run follow-up plans. Use for any task involving the `kit` CLI, manifests (.kit files), Kit providers, or Kit plans."
---

# Using Kit

Kit is a **provider-driven code generator** you run inside your project. Kit
itself is a small, generic shell; all knowledge about *your* stack lives in
**providers** you (or a library) supply. A provider knows what components exist
in a given area of your codebase and how to make more of them. Kit orchestrates
discovery, validation, deterministic file generation, and resumable LLM
follow-up **plans**.

Component types are whatever your providers say they are — for example:

- a SvelteKit provider: routes, pages, remote functions, server hooks, tables
- a TUI provider: widgets, settings, command-palette entries
- a frontend provider: components, stories, pages

Kit does not care about any specific framework. It only cares what components
exist and how to make more of them.

```diagram
╭──────────╮  scans cwd  ╭───────────╮  types()/components()  ╭────────────╮
│   kit    │────────────▶│ providers │───────────────────────▶│ components │
│ (shell)  │             ╰─────┬─────╯                         ╰────────────╯
╰────┬─────╯                   │ create(spec, env)
     │ generate / manifest     ▼
     │                  deterministic files (env.createFile/editFile)
     ▼                         + kit.Event.plan(...) for LLM follow-up
 records plans (resumable, ~/.cache/kit)
```

**Invocation.** This skill writes commands as `kit <command>` — substitute
however Kit is invoked in your project (e.g. a `kit` binary on `PATH`, or
`bun run kit <command>` in a Kit checkout). Start with `kit help` for the full
command list. Kit discovers providers by scanning the **current working
directory**, so run it from your project root.

## Golden rules

These apply both when you *use* Kit and when you *write providers* for it.

1. **Domain logic lives in providers; Kit stays generic.** Model each area of
   your project as a provider with component types. Don't reach around Kit — add
   or extend a provider.
2. **Prefer manifests over `generate`** for anything non-trivial. Manifests are
   declarative, reviewable, validated against live provider schemas, and go
   through `check → plan → apply`.
3. **Generation is deterministic first, LLM second.** A provider's `create()`
   writes the ~80% skeleton deterministically, then yields ONE
   `kit.Event.plan(...)` for the remaining work. Keep those phases separate.
4. **Never write files or spawn generation side effects directly in a
   provider.** Call `env.createFile()` / `env.editFile()` and `yield` the event
   they return. Use `env.spawn()` / `env.exec()` inside `create()` so dry-run
   mode can skip side effects; use `env.dryRun` when generation logic must
   branch.
5. **Use Kit value objects, not string ops.** `kit.FileURI` for paths,
   `kit.Identifier` for hierarchical ids, `kit.spawn()` for read-only discovery
   subprocesses, `kit.Type` (TypeBox) for schemas. Never splice paths or split
   ids by hand.
6. **Schemas are the source of truth.** Help text, manifest vocabulary, and CLI
   options are all generated from `type.schema()` descriptions/examples. Put a
   human-readable `description` on every schema field. Don't duplicate
   descriptions in hardcoded maps.
7. **Discover the API at runtime, don't guess.** The injected `kit` and `env`
   objects are introspectable. Use the REPL (`kit repl`) and `.methods()` /
   `.method(name).signature()` / `.source()`.
8. **Generation can be slow — run `kit generate ...` in a background/tmux
   task.** Keep all other `kit ...` subcommands in the foreground.

## Common workflows

### Discover what exists in your project
```sh
kit provider list                 # providers Kit found in cwd
kit component list                 # every component (provider.id form)
kit component list <prefix>        # filter by hierarchical prefix, e.g. `route`
kit component show <id>            # details + backing files
```

### Author + apply a manifest (preferred path)
```sh
kit manifest vocabulary            # valid forms per live type, with field docs
kit manifest vocabulary --provider <name> --json
kit manifest check   file.kit      # validate only (no provider run)
kit manifest plan    file.kit      # dry-run: "Would create/edit ..." + plan preview
kit manifest apply   file.kit      # write files, then run follow-up plans (default)
kit manifest apply   file.kit --skip-plans  # write files only, leave plans pending
kit manifest apply - < file.kit    # read manifest from stdin
```
Always go `check → plan → apply`. As of kit 0.3.0 `apply` writes the
deterministic skeleton and then runs the follow-up plans automatically; pass
`--skip-plans` to stop after the skeleton and run the agent steps later with
`kit plan`.

### Generate a single component
```sh
kit generate <provider> <type> [--field value ...] [-n|--dry-run] [--intent "..."]
```
`generate` is the CLI surface (positional/flag args → `type.parse`); `manifest`
is the declarative surface. Both call the same provider `type.create(spec, env)`.

### Manage plans (resumable follow-up work)
```sh
kit plan list      # cached plans + progress (e.g. 1/3)
kit plan status    # currently running
kit plan show <id> # steps, current step (→), files, verify commands
kit plan clear     # remove completed cached plans
```

### Inspect the runtime (REPL)
```sh
kit repl new                        # start a session, prints its id + welcome
kit repl do 'kit.methods().map(m => m.signature())'
kit repl do 'kit.method("spawn").source()'
kit repl ls                         # list sessions
kit repl transcript                 # review current session
kit repl stop                       # stop the server (do this after editing source)
```

## Writing your own provider

To teach Kit about a new component family in your project, add a provider that
Kit discovers when it scans the cwd. Read `reference/providers.md` for the
contract and `reference/example-provider.md` for a complete, verbatim provider
you can imitate. In short, a provider default-exports `provider(kit)` and returns
an object that can `name()` itself, list `types()` and existing `components()`,
and `create(spec, env)` new ones.

## References

Read these when the summary above is not enough:

- `reference/providers.md` — the provider & component-type contract, plus Kit's
  built-in providers as worked examples. Read before writing or editing a
  provider.
- `reference/example-provider.md` — a complete, verbatim provider (`kit-command`)
  with annotations. The canonical, self-contained template to imitate. CLIs are a
  familiar domain, so it transfers well to your own component families.
- `reference/manifests.md` — the manifest language (TCL-ish surface, IR,
  TypeBox↔TCL mapping, strings/records/arrays/intent) and the check/plan/apply
  pipeline.
- `reference/repl-and-runtime.md` — the introspectable `kit`/`env` API, FileURI,
  Event, Identifier, and how to discover APIs live instead of guessing.
- `reference/examples.md` — copy-pasteable manifest and provider examples with
  real captured command output.

## Known sharp edges

- **Run Kit from the right directory.** Providers are discovered from
  `<repo-root>/providers/`, `<repo-root>/kit/providers/`,
  `<repo-root>/.kit/providers/`, and `<cwd>/.kit/providers/` (repo root = git
  toplevel). Run from a non-repo directory or put providers elsewhere and
  `provider list` is empty and nothing generates.
- **The instance name comes from the target identifier, not `--name`.**
  `kit generate kit-command component.greet` sets `name=greet`;
  `kit generate kit-command component` (no instance) reports `name is required`.
  When a schema marks a field `cli: false` (as `kit-command` does for `name`),
  that field cannot be passed as a flag: `--name greet` is rejected with a clean
  `Unknown option '--name'.` error. Use the identifier form or a manifest.
- **`manifest apply` runs follow-up plans by default** (kit 0.3.0+): it writes the
  deterministic skeleton and then executes the LLM steps. Pass `--skip-plans` to
  stop after the skeleton and run them later with `kit plan`.
