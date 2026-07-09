# REPL & Runtime API

Kit's `kit` and `env` objects are **introspectable** (Ruby-style runtime method
discovery). When writing or debugging providers, discover the API live instead
of guessing helper shapes or importing internals.

## Introspectable API

Added to objects via `Introspectable.includeInObject` and to classes via
`Introspectable.includeIn` (implemented in `src/introspectable.js` in a Kit
checkout). When an introspectable object has class-valued properties, those
classes are made introspectable too, so `kit.AmpAgentRunner.instanceMethods()`
works without separately registering the class.

On any introspectable **object** (like `kit` or `env`):
```js
kit.methods()             // array of Method, sorted by name
kit.method('spawn')       // one Method (throws if unknown)
kit.respondsTo('spawn')   // boolean
```

On an introspectable **class** (e.g. `kit.ManifestRunner`, `kit.FileURI`):
```js
kit.FileURI.instanceMethods()   // array of Method
kit.FileURI.staticMethods()
kit.FileURI.methods()           // { instance: [...], static: [...] }
```
> Note: on a plain object `kit.methods()` returns a flat array; on a class it
> returns `{ instance, static }`. `kit` itself is an object, so
> `kit.methods()` is a flat array.

A `Method` exposes:
```js
m.name             // 'spawn'
m.signature()      // 'spawn(command, options)'
m.parameterNames() // ['command', 'options']
m.doc()            // registered doc string, or ''
m.documentation()  // alias for doc()
m.source()         // full Function#toString()
m.function         // the original function
m.owner            // object/prototype where it was found
```

Register docs explicitly; runtime `Function#toString()` does not reliably keep
leading JSDoc comments after Bun loads or compiles modules. The metadata is
stored on the function being documented, not in a side registry:

```js
kit.Introspectable.document(MyClass, {
	instance: { create: 'Creates a component and yields file/plan events.' },
	static: { from: 'Parses a value into a MyClass instance.' },
})

kit.Introspectable.document(myObject, {
	run: 'Runs the object.'
})

myObject.run[kit.Introspectable.docSymbol] // 'Runs the object.'
```

## The REPL

A persistent Unix-socket REPL with per-session contexts (`kit` in scope; `env`
too where relevant). `await` is supported.

```sh
bun run kit repl new          # create a session, prints id + welcome (lists available objects)
bun run kit repl ls           # list session ids
bun run kit repl do '<code>'  # evaluate in the current session
bun run kit repl transcript   # replay the current session
bun run kit repl stop         # stop the server and remove its socket (use after editing source)
```

Verified examples:
```sh
bun run kit repl do 'kit.methods().slice(0,8).map(m => m.signature())'
# => [ 'AgentRunner()', 'AmpAgentRunner()', 'bestMatch(candidates, raw)', ... ]

bun run kit repl do 'kit.method("spawn").signature()'
# => 'spawn(command, options)'

bun run kit repl do 'kit.parseManifest(`kit-event event { family provider name providerLoaded }`).components.length'
# => 1

bun run kit repl do 'kit.ManifestRunner.instanceMethods().map(String)'
bun run kit repl do 'env.methods().map(m => m.signature())'
```

State lives under `$XDG_CACHE_DIR/kit` (or `$HOME/.cache/kit`): a per-workspace
socket `repl-<hash>.sock` and `session-<hash>`. The workspace hash is derived
from `process.cwd()`, so each project gets its own REPL. `PlanExecutor` state is
also persisted here (`plan/<id>`), which is what makes plans resumable.

> **Gotcha: the REPL is a long-lived server.** The first `kit repl` call spawns a
> background `kit repl serve` process that loads Kit and your providers **once**.
> Later `repl do` calls reuse it, so edits to Kit or provider source are **not**
> reflected until you restart the server: run `kit repl stop` (it shuts the server
> down cleanly and removes the socket); the next `kit repl` call starts a fresh one.
> If REPL output disagrees with a fresh `kit` command, the server is stale — stop it.

## Key runtime values

Exported from `src/index.js` and available on `kit`.

### `kit.FileURI` — path value object
Never manipulate paths as strings. See `src/file_uri.js` in a Kit checkout.
```js
kit.FileURI.fromPath('src/main.js')        // native path (relative → resolved from cwd) → FileURI
kit.FileURI.from('file:///abs/x.js')       // parse an absolute file URL
uri.path()                                  // native filesystem path
uri.toString()                              // 'file:///...'
uri.join('a', 'b')                          // append relative segments (rejects '..' / absolute)
uri.parent()                                // containing dir as FileURI
uri.relativeTo(base)                        // relative path string (throws if not contained)
uri.withExtension('.json') / uri.withoutExtension('.json')
```

`kit.repoRoot()` also returns a `FileURI`, not a string:

```js
const root = await kit.repoRoot()
const providerDir = root.join('providers').path()
```

### `kit.Identifier` — hierarchical component id
Use instead of splitting ids on `.` by hand (e.g. `kit-event.file.fileRead`).
`kit.Identifier.fromString(id)`, `id.parts()`.

### `kit.Event` — structured event registry
See `src/event.js` in a Kit checkout. Constructors return validated `Event`
objects with `.toJSON()`. Families: file, error, plan, provider, component,
command. Key ones:
```js
kit.Event.fileCreated(path)  kit.Event.fileEdited(path)  kit.Event.fileRead(path)
kit.Event.plan(instructions, steps, fields)
kit.Event.error(message, cause)
kit.Event.commandSpawned/commandOutput/commandExited(...)
```
Add a new event family by generating it: `kit-event event { family X name Y }`.

### `kit.Type` — TypeBox
Re-exported `@sinclair/typebox`. Build schemas at every external/runtime
boundary; always attach `description` (and `examples`).

### `kit.spawn(command, options)`
Runs subprocesses as an event stream (`command.spawned/output/exited`) so
process IO stays observable. Use this for read-only discovery commands and Kit
internals that should always execute. Inside provider `create()` methods, prefer
`env.spawn()` / `env.exec()` so `kit generate -n` and `kit manifest plan` can
report the command without running it.

### `env` — generation environment
`env` is passed to provider `create(spec, env)` methods. It is introspectable,
has a `dryRun` boolean, and exposes:

```js
env.dryRun                         // true for generate -n / manifest plan
await env.createFile(path, source) // returns file.created; no write in dry-run
await env.editFile(path, edit)     // returns file.edited; no write in dry-run
await env.readFile(path)           // generation read through FileURI handling
for await (const event of env.spawn(['cmd'])) yield event
const result = await env.exec(['cmd']) // { code, stdout, stderr, events }
```

Use `env.spawn()` for generation-time side effects. In dry-run mode it yields
`command.spawned` and a successful `command.exited` event without launching the
process. Use `env.exec()` when you need collected stdout/stderr; yield
`result.events` if callers should see the command events.

### `kit.parseArgs(config)` / `kit.parseArgsOptionsFromSchema(schema)`
`parseArgsOptionsFromSchema` turns a TypeBox object schema into `node:util`
parseArgs options (skipping `kit: { cli: false }` fields). `parseArgs` runs `node:util`
parseArgs but converts argument-parsing failures into `kit.UserError`, so a bad
flag produces a clean one-line CLI error instead of a stack trace. Together they
are how `type.parse(argv)` maps the CLI surface onto the same schema the manifest
uses — and a provider's `parse()` gets friendly errors for free.

### `kit.UserError`
Throw this for any user-facing failure — e.g. a `create()` that cannot generate,
or a type that only lists existing components. Kit prints `error.message` with no
stack trace and exits non-zero. Do **not** throw plain `Error` for bad user
input. (CLI argument parse errors are already converted to `UserError` by
`kit.parseArgs`, so you rarely need to do this for parsing.)

### Other useful exports
`kit.discoverProviders()`, `kit.discoverComponents()`,
`kit.discoverComponentRecords()`, `kit.inspectComponent()`,
`kit.loadProvider(path)`, `kit.repoRoot()` (returns `FileURI`), `kit.PlanExecutor`,
`kit.ManifestResolver`, `kit.ManifestRunner`, `kit.ManifestVocabulary`,
`kit.parseManifest`, `kit.TableFormatter`, `kit.PlanFormatter`,
`kit.EphemeralStateStore`, `kit.PersistentStateStore`.
