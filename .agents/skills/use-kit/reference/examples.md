# Worked Examples

Real commands and captured output from a Kit checkout, using Kit's **built-in**
providers (`kit-command`, `kit-event`, `kit-provider`) as concrete, runnable
examples. In your own project the provider and type names differ — they come from
whatever providers you add — but the workflow is identical.

Commands here are shown as `bun run kit ...` (how Kit is invoked from a checkout);
substitute your project's invocation, e.g. a `kit` binary on `PATH`. Absolute
paths shown are illustrative and will differ.

## 1. Explore the tool

```sh
bun run kit help
bun run kit provider list
```
```
Provider      Path
kit-event     file:///…/providers/kit-event/index.js
kit-provider  file:///…/providers/kit-provider/index.js
kit-command   file:///…/providers/kit-command/index.js
```
```sh
bun run kit component list kit-event.file
```
```
Component                   Description
kit-event.file.fileCreated  File was created
kit-event.file.fileRead     File was read
kit-event.file.fileRemoved  File was removed
kit-event.file.fileEdited   File was edited
```

## 2. A CLI-command manifest, end to end

`greet.kit`:
```kit
kit-command component {
	name greet
	description "Say hello to someone"
	arg who
}
```

Validate → preview → apply:
```sh
bun run kit manifest check greet.kit
# greet.kit is valid

bun run kit manifest plan greet.kit
# Would create …/src/commands/greet.js
# Would edit   …/src/main.js
# Plan (pending)
# Status: pending
# Instructions:
#   Finish generated greet command
# Steps:
#     1. fill-in-command
#      Use the configured agent to finish generated command implementation
#      Files: file:///…/src/commands/greet.js

bun run kit manifest apply greet.kit
# Created …/src/commands/greet.js
# Edited  …/src/main.js
# Plan (pending)  …
```

Generated `src/commands/greet.js` (deterministic skeleton):
```js
import { defineCommand } from '../cli.js'

/**
 * Command group for Say hello to someone.
 *
 * @example
 * await createCLI([greet]).run(['greet'])
 */
const greet = defineCommand({
	name: 'greet',
	description: "Say hello to someone",
	async run() {
		throw new Error('Command greet is not implemented yet')
	},
})

export default greet
```
The plan's `fill-in-command` step is what an LLM finishes (run with
`manifest apply greet.kit --run-plans`, or `kit generate`'s auto-execution).

`src/main.js` is edited to import and register the command in `createCLI([...])`.
Afterwards `bun run kit component list | grep greet` shows
`kit-command.greet  Say hello to someone`.

## 3. Add a new event family

`event.kit`:
```kit
kit-event event {
	family workspace
	name workspaceOpened
}
```
```sh
bun run kit manifest apply event.kit
# Created …/src/events/workspace.js      (new family: schema + workspaceOpenedEvent builder)
# Edited  …/src/event.js                 (imports, ...workspaceSchemas, Event.workspaceOpened())
```
If the family already exists, `create()` uses an **idempotent** `editFile` that
appends the schema/builder only when missing — safe to re-apply.

## 4. Scaffold a new provider

`provider.kit`:
```kit
kit-provider provider {
	name demo-widget
	description "Generate demo widgets"

	intent {
		List and generate demo widgets under widgets/.
	}
}
```
```sh
bun run kit manifest apply provider.kit --run-plans
```
This writes `providers/demo-widget/index.js` from the skeleton template, then
runs a 3-step plan: **analyze the domain → implement the provider → verify with
`bun run kit provider list`**. The skeleton already default-exports
`provider(kit)` and stubs `types()`, `components()`, and a throwing `create()`.

## 5. Inspect the runtime with the REPL

```sh
bun run kit repl new
bun run kit repl do 'kit.methods().slice(0,8).map(m => m.signature())'
# => [ 'AgentRunner()', 'AmpAgentRunner()', 'bestMatch(candidates, raw)', 'CLI()', ... ]

bun run kit repl do 'kit.method("spawn").signature()'
# => 'spawn(command, options)'

bun run kit repl do 'kit.parseManifest(`kit-event event { family provider name providerLoaded }`).components.length'
# => 1
```
When writing a provider, use the same trick to learn `env`:
```sh
bun run kit repl do 'env.methods().map(m => m.signature())'
```

## 6. Manage plans

```sh
bun run kit plan list
# ID                                    Progress  Description
# 2d9cf02e-…                            0/1       Finish generated manifest command
# 51f4e607-…                            3/3       Finish generated real-env-test provider

bun run kit plan show 2d9cf02e-…
# Plan 2d9cf02e-…
# Status: running
# Steps:
#   → 1. fill-in-command
#      Use the configured agent to finish generated command implementation
#      Files: file:src/commands/manifest.js

bun run kit plan clear   # drop completed cached plans
```

## Gotchas (verified)

- **`generate` validates the spec first, then generates.** Missing/invalid
  fields report a readable error instead of crashing inside a provider:
  ```sh
  bun run kit generate kit-command component
  # Cannot generate kit-command component:
  #   name is required — Kebab-case command or subcommand name
  #
  # Run `kit generate kit-command component --help` to see all fields.

  bun run kit generate kit-command component.Bad_Name
  # Cannot generate kit-command component:
  #   name is invalid (Expected string to match '^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$') — Kebab-case command or subcommand name
  ```
  The instance name comes from the target identifier
  (`component.greet` → `name=greet`). Supply it, or use a manifest.

- **Output location is a provider-owned field.** `kit-provider` declares an
  optional `project` field (default `<repo-root>/providers`) for where a new
  provider is scaffolded — it works the same on both surfaces:
  ```sh
  # generate: --project comes from the kit-provider schema
  bun run kit generate kit-provider provider.demo-widget --description "Demo widgets" --project /tmp/out -n
  # Would create /tmp/out/demo-widget/index.js

  # default (no --project) → <repo-root>/providers
  bun run kit generate kit-provider provider.demo-widget --description "Demo widgets" -n
  # Would create /…/providers/demo-widget/index.js
  ```
  ```kit
  # manifest: `project` is just another field
  kit-provider provider {
  	name demo-widget
  	description "Demo widgets"
  	project .kit/providers
  }
  ```
  It is **not** a global `generate` flag, and other providers that write to fixed
  locations (e.g. `kit-command` → `src/commands/`) don't declare it.

- **Preview before writing.** `check` never runs providers; `plan` uses a dry-run
  env (no disk writes); `apply` writes but does not run plans unless you pass
  `--run-plans`.
