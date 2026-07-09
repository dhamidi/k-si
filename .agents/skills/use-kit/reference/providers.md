# Writing Kit Providers

A provider teaches Kit about one component family. It lists existing components,
generates deterministic scaffolds, and emits structured events/plans — without
leaking framework details into Kit core.

Providers live at `providers/<name>/index.js` and are discovered by scanning the
current working directory. Each must `export default function provider(kit)`.

## The contract

### Provider object
```js
export default function provider(kit) {
	return new MyProvider(kit)
}

class MyProvider {
	constructor(kit) { this.kit = kit }
	name() { return 'my-provider' }         // stable provider id
	async *types() { yield new MyType(this.kit) }        // component types it can create
	async *components() { /* yield existing component objects */ }
	create(spec, env) {                      // delegate to the matching type
		return new MyType(this.kit).create(spec, env)
	}
}
```

### Component type object
```js
class MyType {
	constructor(kit) { this.kit = kit }
	id() { return 'component' }              // type id (e.g. 'route', 'table', 'event')
	description() { return 'User-facing type description' }

	schema() {                               // TypeBox schema — the source of truth
		const { Type } = this.kit
		return Type.Object({
			name: Type.String({
				description: 'Generated component name',   // becomes help + vocabulary text
				examples: ['widget'],
				pattern: '^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$',
			}),
		})
	}

	parse(argv) {                            // CLI surface (kit generate ...)
		return this.kit.parseArgs({
			args: argv,
			options: this.kit.parseArgsOptionsFromSchema(this.schema()),
			strict: true,
			allowPositionals: true,
		})
	}

	describe(spec) { return spec.name }      // concise description of a specific instance

	async *create(spec, env) {               // manifest/apply surface
		const path = this.kit.FileURI.fromPath('src/x.js').path()
		yield await env.createFile(path, template(spec))
		if (spec.intent !== undefined) {
			yield this.kit.Event.plan(`Implement ${spec.name}`, [{
				id: 'implement',
				instructions: `Use the configured agent to implement ${spec.name}`,
				files: [path],
			}], { intent: spec.intent })
		}
	}
}
```

### Component object (for `components()` / `component show`)
```js
class MyComponent {
	provider() { return 'my-provider' }      // owning provider name
	id() { return 'some.hierarchical.id' }   // id within the provider
	description() { return '...' }           // derive from the SOURCE object, not a hardcoded fallback
	inspect() { return { name: this.id(), files: [this.path] } }  // structured fields
}
```

Both `generate` and `manifest apply` ultimately call `type.create(spec, env)` —
one code path, two surfaces.

## Rules (from PROVIDERS.md + AGENTS.md)

- **Use only the injected `kit` object + standard Bun APIs.** Do not import Kit
  internals from provider code. Inspect unfamiliar APIs dynamically:
  `kit.methods()`, `kit.method(name).signature()`, `kit.method(name).source()`,
  `env.methods()`, `env.method(name).parameterNames()`.
- **Values, not strings:** `kit.FileURI` for paths, `kit.Identifier` for ids,
  `env.spawn()` for generation-time subprocesses, `kit.spawn()` for read-only
  discovery subprocesses that should always run, `kit.Type` for schemas with
  `description`.
- **Deterministic first, plan second.** Emit files deterministically; add ONE
  `kit.Event.plan(...)` for the LLM remainder. Keep plan wording specific to the
  generated component, not copied from another provider.
- **Never write files directly or spawn generation side effects directly.** Use
  `env.createFile()` / `env.editFile()` and `yield` their events; use
  `env.spawn()` / `env.exec()` for generation commands so dry-run mode can skip
  the side effect.
- **Signal failures with `kit.UserError`, not plain `Error`.** If a type can't be
  created (or only lists existing components), throw `this.kit.UserError(...)`;
  Kit prints the message with no stack trace and exits non-zero. Argument parse
  errors are already converted to `UserError` by `kit.parseArgs`.
- **Descriptions come from the source object being modeled**, so provider output
  stays synced with the code it describes. No fallbacks like
  `Commander command foo`.
- **Custom output when the generic table is noisy:** implement
  `formatComponentsList(records, output)` and
  `formatComponentShow(componentName, record, output)`. Use `kit.TableFormatter`
  for dense tables; fix `TableFormatter` rather than patching padding per
  provider.
- **Code search/analysis:** prefer `ast-grep` for JS/TS/Svelte structure; one
  broad scan per file/dir over many small spawns. Regex only for simple strings
  after ast-grep selected the node.
- **Performance:** cache per-file discovery during a listing pass; avoid one
  process per component/field. `component list`/`show` must be fast enough to run
  foreground.

## The env object

`env` (from `kit.createFileEnv()`) is what `create(spec, env)` writes through.
It is introspectable and exposes:

- `dryRun` → `true` during `kit generate -n` and `kit manifest plan`.
- `createFile(path, content)` → writes the file, returns a `file.created` event.
- `editFile(path, edit)` → reads the file, applies `edit(source) => nextSource`
  (a function) or leaves it unchanged, writes it, returns a `file.edited` event.
- `readFile(path)` → reads a file through Kit's `FileURI` path handling.
- `spawn(command, options)` → streams command events; dry-run yields
  `command.spawned` / successful `command.exited` without running the command.
- `exec(command, options)` → returns `{ code, stdout, stderr, events }`; use it
  when generation needs collected command output.

`kit manifest plan` and `kit generate -n` swap in a **dry-run env** that returns
the same file events without touching disk and command events without spawning
processes. Do not sniff method arity to detect dry-run; read `env.dryRun`.

## Follow-up plans

`kit.Event.plan(instructions, steps, fields)` builds a plan event. Step shape
(see `src/events/plan.js`):

```js
{
	id: 'implement-component',            // optional step id (used to namespace agent output)
	instructions: 'Use the configured agent to implement …',
	files: ['file:///abs/path.js'],       // optional; FileURI-normalized automatically
	agent: {                              // optional; presence means "run an agent for this step"
		prompt: 'Full self-contained instructions for the agent',
		command: ['bun', 'test'],         // optional deterministic command
	},
	verifyWithCommand: 'bun run kit provider list',  // optional; step passes only if exit 0
}
```
Pass common fields via the third arg, e.g. `{ intent: spec.intent }`. Only
`intent` is a Kit-core spec field; everything else is provider schema.

`PlanExecutor` (`src/plan_executor.js`) runs steps in order, persists resumable
state under `~/.cache/kit` (see REPL reference), retries a step up to 2× when a
`verifyWithCommand` fails, and namespaces agent updates as `stepId:kind`.

## Kit's built-in providers (worked examples)

Kit ships three providers that generate Kit's *own* building blocks — Kit uses
them to build itself. You will rarely invoke these in an application project, but
they are the canonical, battle-tested examples of the provider contract above.
Study them (and imitate them) when writing providers for your own component
families.

| Provider       | Type id      | Generates                              | Notable technique |
|----------------|--------------|----------------------------------------|-------------------|
| `kit-event`    | `event`      | `src/events/<family>.js` schema + builder, registers in `src/event.js` | idempotent `editFile` that appends to an existing family and wires imports/`Event.<name>()` |
| `kit-provider` | `provider`   | `providers/<name>/index.js` skeleton   | multi-step plan: analyze domain → implement → verify with `bun run kit provider list` |
| `kit-command`  | `component`  | `src/commands/<name>.js`, registers in `src/main.js` | special-cases `component show`; group vs leaf command templates |

**Read [example-provider.md](example-provider.md)** for the complete, verbatim
`kit-command` source with annotations — it is the canonical template to imitate.
In a Kit checkout the other two live at `providers/kit-event/index.js` and
`providers/kit-provider/index.js`.

### Discovery pattern used by all three
```js
async *components() {
	for await (const path of new Glob('src/events/*.js').scan({ cwd: process.cwd() })) {
		const module = await import(this.kit.FileURI.fromPath(path).toString())
		// read exported schemas / default command / etc. and yield component objects
	}
}
```
`kit-event` reads exported `*Schemas`; `kit-command` reads the default-exported
command and its `.commands`; `kit-provider` derives the name from the directory.
