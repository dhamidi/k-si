import { Glob } from 'bun'

/**
 * Kit provider for käsi's domain primitives (docs/15-tactical-patterns.md):
 *
 * - module        a domain package: module.go + model slice + msg/ contract
 * - message       message_<tag>.go: tag const + payload + handler + registration
 * - command       command_<tag>.go: tag const + payload + constructor + effect
 * - model         model_<name>.go: a model slice / business object
 * - subscription  subscription_<name>.go: state -> set of running sources
 * - view          web/view_<name>.vue + view_<name>.go: an htmlc view and its View struct
 * - form          web/form_<name>.go: bind + validate + construct the message; re-renders with errors
 *
 * Discovery is structural (ast-grep over Go source); generation emits the
 * canonical shapes from the pattern book, so generated code and documented
 * code are the same code.
 */

const RUNTIME_PACKAGE = 'runtime'
const RESERVED_DIRS = new Set([
	'runtime', 'cmd', 'testlang', 't', 'providers', 'docs', 'vendor',
	'web', 'store', 'mime', 'control', 'msg', // packages that are not domain modules ([09])
])

class KasiProvider {
	constructor(kit) {
		this.kit = kit
	}

	name() {
		return 'kasi'
	}

	async *types() {
		yield new ModuleType(this.kit)
		yield new MessageType(this.kit)
		yield new CommandType(this.kit)
		yield new ModelType(this.kit)
		yield new SubscriptionType(this.kit)
		yield new ViewType(this.kit)
		yield new FormType(this.kit)
	}

	async *components() {
		const scan = await scanRepository(this.kit)

		for (const module of scan.modules) {
			yield new KasiComponent({
				kind: 'module',
				id: `module.${module.name}`,
				description: module.description ?? `Domain module ${module.name}`,
				files: module.files,
				details: { module: module.name },
			})
		}

		for (const message of scan.messages) {
			yield new KasiComponent({
				kind: 'message',
				id: `message.${message.module}.${message.tag}`,
				description: message.description ?? `Runtime message "${message.tag}"`,
				files: message.files,
				details: {
					module: message.module,
					tag: message.tag,
					contract: message.contract,
					handler: message.handler,
				},
			})
		}

		for (const command of scan.commands) {
			yield new KasiComponent({
				kind: 'command',
				id: `command.${command.module}.${command.tag}`,
				description: command.description ?? `Command "${command.tag}"`,
				files: command.files,
				details: {
					module: command.module,
					tag: command.tag,
					contract: command.contract,
					effect: command.handler,
				},
			})
		}

		for (const model of scan.models) {
			yield new KasiComponent({
				kind: 'model',
				id: `model.${model.module}.${model.name}`,
				description: model.description ?? `Model slice object ${model.name}`,
				files: model.files,
				details: { module: model.module },
			})
		}

		for (const subscription of scan.subscriptions) {
			yield new KasiComponent({
				kind: 'subscription',
				id: `subscription.${subscription.module}.${subscription.name}`,
				description: subscription.description ?? `Subscription ${subscription.name}`,
				files: subscription.files,
				details: { module: subscription.module },
			})
		}

		for (const view of scan.views) {
			yield new KasiComponent({
				kind: 'view',
				id: `view.${view.name}`,
				description: view.description ?? `htmlc view ${view.name}`,
				files: view.files,
				details: { render: view.render },
			})
		}

		for (const form of scan.forms) {
			yield new KasiComponent({
				kind: 'form',
				id: `form.${form.name}`,
				description: form.description ?? `Form object ${form.name}`,
				files: form.files,
				details: { message: form.message },
			})
		}

		for (const scenario of scan.scenarios) {
			yield new KasiComponent({
				kind: 'scenario',
				id: `scenario.${scenario.name}`,
				description: scenario.description ?? `Test scenario ${scenario.name}`,
				files: scenario.files,
				details: {},
			})
		}
	}

	async create(spec, env) {
		throw new this.kit.UserError('Use a component type (module, message, command, model, subscription, view, form) to generate')
	}
}

// --- Discovery ---------------------------------------------------------------

async function scanRepository(kit) {
	const scan = { modules: [], messages: [], commands: [], models: [], subscriptions: [], scenarios: [], views: [], forms: [] }

	for await (const path of new Glob('web/form_*.go').scan({ cwd: process.cwd() })) {
		const snake = path.replace(/^web\/form_([a-z0-9_]+)\.go$/, '$1')
		const source = await Bun.file(path).text()

		scan.forms.push({
			name: snake.replaceAll('_', '-'),
			files: [path],
			message: source.match(/msg\.New([A-Za-z0-9]+)\(/)?.[1],
			description: await docComment(path, /\/\/ [A-Za-z0-9]+Form [—-] ?(.+)/),
		})
	}

	for await (const path of new Glob('web/view_*.vue').scan({ cwd: process.cwd() })) {
		const snake = path.replace(/^web\/view_([a-z0-9_]+)\.vue$/, '$1')
		const goFile = `web/view_${snake}.go`
		const files = [path]
		let render

		if (await Bun.file(goFile).exists()) {
			files.push(goFile)
			const source = await Bun.file(goFile).text()
			render = source.includes('RenderFragment(') ? 'fragment' : 'page'
		}

		scan.views.push({
			name: snake.replaceAll('_', '-'),
			files,
			render,
			description: await docComment(path, /<!-- view_[a-z0-9_]+\.vue [—-] ?(.+?)(?: \(docs\/[0-9]+\))? -->/),
		})
	}

	for await (const path of new Glob('t/**/*.test').scan({ cwd: process.cwd() })) {
		scan.scenarios.push({
			name: path.replace(/^t\//, '').replace(/\.test$/, '').replaceAll('/', '.'),
			files: [path],
			description: await docComment(path, /^# (?:t\/\S+ — )?(.+)$/m),
		})
	}

	const goFiles = await glob('*/**.go')

	if (goFiles.length === 0) {
		return scan
	}

	for await (const path of new Glob('*/module.go').scan({ cwd: process.cwd() })) {
		const name = path.split('/')[0]
		if (RESERVED_DIRS.has(name)) continue // runtime/module.go is machinery, not a domain
		scan.modules.push({
			name,
			files: [path],
			description: await docComment(path, /\/\/ Module bundles (.+?)(?: \(docs\/[0-9]+\))?\.?$/m),
		})
	}

	const constants = await tagConstants(kit)
	scan.messages = await registrations(kit, 'HandleMsg', constants)
	scan.commands = await registrations(kit, 'HandleCmd', constants)

	for (const match of await astGrep(kit, 'type $NAME struct { $$$ }')) {
		if (!/\/model_[a-z0-9_]+\.go$/.test(match.file)) continue
		const name = meta(match, 'NAME')
		if (name === 'Model') continue // the module's slice; listed with the module itself
		scan.models.push({
			module: moduleOf(match.file),
			name,
			files: [match.file],
			description: await docComment(match.file, new RegExp(`// ${name} [—-] ?(.+?)(?: \\(docs/[0-9]+\\))?\\.?$`, 'm')),
		})
	}

	for await (const path of new Glob('*/subscription_*.go').scan({ cwd: process.cwd() })) {
		const name = path.replace(/^.*subscription_([a-z0-9_]+)\.go$/, '$1').replaceAll('_', '-')
		scan.subscriptions.push({
			module: moduleOf(path),
			name,
			files: [path],
			description: await docComment(path, new RegExp(`// ${name} [—-] ?(.+)`)),
		})
	}

	return scan
}

/**
 * Maps `<dir>:<ConstName>` to its string value for every Go string constant,
 * so `HandleMsg(mod, msg.CreateTask, …)` resolves to the "create-task" tag.
 */
async function tagConstants(kit) {
	const constants = new Map()

	for (const match of await astGrep(kit, 'const $NAME = $VALUE')) {
		const value = meta(match, 'VALUE')
		if (value === undefined || !/^"[a-z0-9-]+"$/.test(value)) continue
		constants.set(`${dirOf(match.file)}:${meta(match, 'NAME')}`, {
			tag: JSON.parse(value),
			file: match.file,
		})
	}

	return constants
}

async function registrations(kit, method, constants) {
	const found = []

	for (const match of await astGrep(kit, `${RUNTIME_PACKAGE}.${method}($MOD, $TAG, $HANDLER)`)) {
		const module = moduleOf(match.file)
		const resolved = resolveTag(meta(match, 'TAG'), match.file, constants)
		if (resolved === undefined) continue

		const files = [match.file]
		if (resolved.file !== match.file) files.push(resolved.file)

		found.push({
			module,
			tag: resolved.tag,
			contract: resolved.file.includes('/msg/'),
			handler: meta(match, 'HANDLER'),
			files,
			description: await docComment(match.file, /\/\/ "([^"]+)" [—-] ?(.+)/, 2),
		})
	}

	return found.sort((a, b) => a.tag.localeCompare(b.tag))
}

function resolveTag(reference, file, constants) {
	if (reference === undefined) return undefined
	if (/^"[a-z0-9-]+"$/.test(reference)) return { tag: JSON.parse(reference), file }

	const dir = dirOf(file)
	const [qualifier, name] = reference.includes('.') ? reference.split('.') : [undefined, reference]

	if (qualifier === 'msg') return constants.get(`${dir}/msg:${name}`)
	if (qualifier !== undefined) return constants.get(`${qualifier}/msg:${name}`) ?? constants.get(`${qualifier}:${name}`)
	return constants.get(`${dir}:${name}`)
}

/** Runs one broad ast-grep scan and returns structured matches. */
async function astGrep(kit, pattern) {
	let stdout = ''
	let code = 0

	try {
		const events = kit.spawn(['ast-grep', 'run', '--pattern', pattern, '--lang', 'go', '--json=stream', '.'])
		for await (const event of events) {
			const value = event.toJSON()
			if (value.type === 'command.output' && value.stream === 'stdout') {
				stdout += new TextDecoder().decode(Uint8Array.from(value.bytes))
			}
			if (value.type === 'command.exited') {
				code = value.code
			}
		}
	} catch {
		return [] // ast-grep not installed: discovery degrades to "nothing found"
	}

	if (code !== 0) return []

	return stdout
		.split('\n')
		.filter((line) => line.startsWith('{'))
		.map((line) => JSON.parse(line))
		.filter((match) => !match.file.startsWith('providers/'))
}

function meta(match, name) {
	return match.metaVariables?.single?.[name]?.text
}

function moduleOf(path) {
	return path.split('/')[0]
}

function dirOf(path) {
	return path.split('/').slice(0, -1).join('/')
}

async function docComment(path, regex, group = 1) {
	try {
		const source = await Bun.file(path).text()
		const match = source.match(regex)
		return match?.[group]?.trim()
	} catch {
		return undefined
	}
}

async function glob(pattern) {
	const paths = []
	for await (const path of new Glob(pattern).scan({ cwd: process.cwd() })) {
		if (!path.startsWith('providers/')) paths.push(path)
	}
	return paths
}

async function goModulePath(env) {
	try {
		const gomod = await env.readFile('go.mod')
		return gomod.match(/^module (\S+)/m)?.[1] ?? 'kasi'
	} catch {
		return 'kasi'
	}
}

// --- Shared type behavior ----------------------------------------------------

class KasiType {
	constructor(kit) {
		this.kit = kit
	}

	parse(argv) {
		return this.kit.parseArgs({
			args: argv,
			options: this.kit.parseArgsOptionsFromSchema(this.schema()),
			strict: true,
			allowPositionals: true,
		})
	}

	describe(spec) {
		return spec.description ?? this.description()
	}

	/**
	 * `kit generate kasi message.tasks.create-task` arrives as
	 * spec.parent=tasks, spec.name=create-task; manifests say module/tag
	 * explicitly. Both spell the same spec.
	 */
	normalize(spec) {
		const module = spec.module ?? spec.parent
		const name = spec.tag ?? spec.name

		if (module === undefined || !/^[a-z][a-z0-9]*$/.test(module)) {
			throw new this.kit.UserError(`${this.id()}: module must be a lower-case Go package name, got ${JSON.stringify(module)}`)
		}
		if (RESERVED_DIRS.has(module)) {
			throw new this.kit.UserError(`${this.id()}: ${module} is not a domain module`)
		}
		if (name === undefined || !/^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(name)) {
			throw new this.kit.UserError(`${this.id()}: name/tag must be kebab-case, got ${JSON.stringify(name)}`)
		}

		return { ...spec, module, name }
	}

	moduleField() {
		const { Type } = this.kit
		return Type.Optional(
			Type.String({
				description: 'Domain module (Go package) this component belongs to',
				examples: ['tasks', 'email', 'agents'],
				pattern: '^[a-z][a-z0-9]*$',
				cli: false,
			}),
		)
	}

	tagField(description, examples) {
		const { Type } = this.kit
		return Type.Optional(
			Type.String({
				description,
				examples,
				pattern: '^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$',
				cli: false,
			}),
		)
	}

	fieldsField() {
		const { Type } = this.kit
		return Type.Optional(
			Type.Record(
				Type.String({ pattern: '^[a-z][a-z0-9_]*$' }),
				Type.String({ description: 'Go type of the field' }),
				{
					description: 'Payload fields: snake_case name -> Go type; json tags are derived from the names',
					examples: [{ task_id: 'int64', sender: 'string', cc: '[]string' }],
					cli: false,
				},
			),
		)
	}

	descriptionField(examples) {
		const { Type } = this.kit
		return Type.Optional(
			Type.String({
				description: 'One-line, user-facing description; becomes the doc comment',
				examples,
			}),
		)
	}

	/**
	 * Creates a file only if it does not exist yet, so re-applying a manifest
	 * never clobbers code someone has already implemented. An existing file is
	 * reported as read: consulted, left untouched.
	 */
	async *createFresh(env, path, content) {
		if (await Bun.file(path).exists()) {
			yield this.kit.Event.fileRead(path)
			return
		}

		yield await env.createFile(path, content)
	}

	/**
	 * Tags are globally unique — one tag, one owner (docs/01). Regenerating
	 * the same component is fine (createFresh makes it a no-op); the same tag
	 * under a different module or kind is refused.
	 */
	async ensureTagFree(spec, kind) {
		const scan = await scanRepository(this.kit)
		const owners = [
			...scan.messages.map((m) => ({ ...m, kind: 'message' })),
			...scan.commands.map((c) => ({ ...c, kind: 'command' })),
		]
		const clash = owners.find((owner) => owner.tag === spec.name)

		if (clash && (clash.module !== spec.module || clash.kind !== kind)) {
			throw new this.kit.UserError(
				`tag "${spec.name}" is already owned by ${clash.kind} ${clash.module}.${clash.tag} (${clash.files[0]}); tags are globally unique (docs/01)`,
			)
		}
	}

	/**
	 * Wires a new module into the one assembly (docs/01: main.go is THE
	 * assembly point). Before stage zero exists there is nothing to wire and
	 * that is fine; once cmd/kasi/main.go exists, forgetting to wire is the
	 * kind of silent divergence this provider exists to prevent.
	 */
	async *wireIntoAssembly(spec, env, gomod) {
		const mainPath = 'cmd/kasi/main.go'

		if (!(await Bun.file(mainPath).exists())) {
			return
		}

		const source = await env.readFile(mainPath)
		const wired = wireModule(source, spec.name, gomod)

		if (wired === source) {
			if (source.includes(`${spec.name}.Module(`)) {
				yield this.kit.Event.fileRead(mainPath) // already wired
			} else {
				yield this.kit.Event.error(
					`could not wire ${spec.name} into ${mainPath}: expected an import ( block and a runtime.New( assembly (docs/01)`,
				)
			}
			return
		}

		yield await env.editFile(mainPath, () => wired)
	}

	async *registerInModule(spec, env, call) {
		const modulePath = `${spec.module}/module.go`

		if (!(await Bun.file(modulePath).exists())) {
			if (env.dryRun) {
				// In a real apply the module generated earlier in the manifest
				// exists by now; report the edit that would happen.
				yield this.kit.Event.fileEdited(modulePath)
				return
			}

			yield this.kit.Event.error(
				`${modulePath} does not exist; generate the module first: kit generate kasi module.${spec.module}`,
			)
			return
		}

		yield await env.editFile(modulePath, (source) => {
			if (source.includes(call)) return source
			return source.replace(/\n(\treturn mod\n})/, `\n\t${call}\n$1`)
		})
	}

	plan(spec, title, files, prompt) {
		return this.kit.Event.plan(
			title,
			[
				{
					id: 'fill-in',
					instructions: `Implement the generated skeleton according to the intent`,
					files,
					agent: { prompt },
				},
			],
			{ intent: spec.intent },
		)
	}
}

// --- module ------------------------------------------------------------------

class ModuleType extends KasiType {
	id() {
		return 'module'
	}

	description() {
		return 'A käsi domain module: module.go + model slice + msg/ contract package (docs/15)'
	}

	schema() {
		const { Type } = this.kit
		return Type.Object({
			name: Type.Optional(
				Type.String({
					description: 'Module name: a lower-case Go package / directory name',
					examples: ['tasks', 'email', 'agents'],
					pattern: '^[a-z][a-z0-9]*$',
					cli: false,
				}),
			),
			description: this.descriptionField(['Task lifecycle and workspaces']),
		})
	}

	normalize(spec) {
		const name = spec.name ?? spec.module
		if (name === undefined || !/^[a-z][a-z0-9]*$/.test(name) || RESERVED_DIRS.has(name)) {
			throw new this.kit.UserError(`module: name must be a fresh lower-case package name, got ${JSON.stringify(name)}`)
		}
		return { ...spec, name, module: name }
	}

	async *create(rawSpec, env) {
		const spec = this.normalize(rawSpec)
		const gomod = await goModulePath(env)
		const what = spec.description ?? `the ${spec.name} domain`

		yield* this.createFresh(env, `${spec.name}/module.go`, moduleTemplate(spec.name, what, gomod))
		yield* this.createFresh(env, `${spec.name}/model_${spec.name}.go`, modelSliceTemplate(spec.name))
		yield* this.createFresh(env, `${spec.name}/msg/doc.go`, contractDocTemplate(spec.name))
		yield* this.wireIntoAssembly(spec, env, gomod)

		if (spec.intent !== undefined) {
			yield this.plan(
				spec,
				`Flesh out the ${spec.name} module`,
				[`${spec.name}/module.go`, `${spec.name}/model_${spec.name}.go`],
				`Intent: ${spec.intent}

The ${spec.name} module skeleton exists. Add its edges to the Edges struct
(real + simulated twins, docs/12), define its model slice, and wire the module
into the assembly in cmd/kasi/main.go (docs/01 "Modules and assembly").
Follow docs/15-tactical-patterns.md exactly. Do not refactor unrelated code.`,
			)
		}
	}
}

// --- message -----------------------------------------------------------------

class MessageType extends KasiType {
	id() {
		return 'message'
	}

	description() {
		return 'A runtime message: tag + payload + pure handler + registration (docs/15)'
	}

	schema() {
		const { Type } = this.kit
		return Type.Object({
			module: this.moduleField(),
			tag: this.tagField('Imperative, kebab-case message tag', ['create-task', 'finish-agent-run']),
			contract: Type.Optional(
				Type.Boolean({
					description: 'Other domains send this: payload + constructor go into <module>/msg (docs/15)',
					examples: [true],
				}),
			),
			fields: this.fieldsField(),
			description: this.descriptionField(['sent by email/route-email; creates the Task']),
		})
	}

	async *create(rawSpec, env) {
		const spec = this.normalize(rawSpec)
		await this.ensureTagFree(spec, 'message')
		const gomod = await goModulePath(env)
		const snake = snakeCase(spec.name)
		const messageFile = `${spec.module}/message_${snake}.go`
		const files = [messageFile]

		if (spec.contract) {
			const contractFile = `${spec.module}/msg/${snake}.go`
			yield* this.createFresh(env, contractFile, contractMessageTemplate(spec, gomod))
			yield* this.createFresh(env, messageFile, contractHandlerTemplate(spec, gomod))
			files.push(contractFile)
		} else {
			yield* this.createFresh(env, messageFile, messageTemplate(spec, gomod))
		}

		yield* this.registerInModule(spec, env, `register${pascalCase(spec.name)}(mod)`)

		if (spec.intent !== undefined) {
			yield this.plan(
				spec,
				`Implement the "${spec.name}" handler`,
				files,
				`Intent: ${spec.intent}

Implement handle${pascalCase(spec.name)} in ${messageFile}. The handler is
pure: no I/O, no time, no randomness — everything it needs is on the payload
or derivable from the View and meta (docs/15, docs/01). Return the updated
${spec.module} slice and any commands. Do not refactor unrelated code.`,
			)
		}
	}
}

// --- command -----------------------------------------------------------------

class CommandType extends KasiType {
	id() {
		return 'command'
	}

	description() {
		return 'A command: tag + payload + constructor + effect over edges (docs/15)'
	}

	schema() {
		const { Type } = this.kit
		return Type.Object({
			module: this.moduleField(),
			tag: this.tagField('Imperative, kebab-case command tag', ['send-email', 'spawn-agent-run']),
			contract: Type.Optional(
				Type.Boolean({
					description: 'Other domains return this command: constructor goes into <module>/msg (docs/15)',
					examples: [true],
				}),
			),
			fields: this.fieldsField(),
			emits: this.tagField('Message tag the effect emits when the work is done', ['mark-email-sent']),
			description: this.descriptionField(['transmit one pending outbox row via the mail edge']),
		})
	}

	async *create(rawSpec, env) {
		const spec = this.normalize(rawSpec)
		await this.ensureTagFree(spec, 'command')
		const gomod = await goModulePath(env)
		const snake = snakeCase(spec.name)
		const commandFile = `${spec.module}/command_${snake}.go`
		const files = [commandFile]

		if (spec.contract) {
			const contractFile = `${spec.module}/msg/${snake}.go`
			yield* this.createFresh(env, contractFile, contractCommandTemplate(spec, gomod))
			yield* this.createFresh(env, commandFile, contractEffectTemplate(spec, gomod))
			files.push(contractFile)
		} else {
			yield* this.createFresh(env, commandFile, commandTemplate(spec, gomod))
		}

		yield* this.registerInModule(spec, env, `register${pascalCase(spec.name)}(mod)`)

		if (spec.intent !== undefined) {
			yield this.plan(
				spec,
				`Implement the "${spec.name}" effect`,
				files,
				`Intent: ${spec.intent}

Implement ${camelCase(spec.name)}Effect in ${commandFile}. Effects see edges
and payload only — never the model. Results leave only as emitted messages
built with constructors${spec.emits ? ` (emit "${spec.emits}" on success)` : ''};
timestamps come from the Clock edge; errors defer to reconciliation (docs/15).
Do not refactor unrelated code.`,
			)
		}
	}
}

// --- model -------------------------------------------------------------------

class ModelType extends KasiType {
	id() {
		return 'model'
	}

	description() {
		return 'A model slice object: plain values, typed ids, no I/O (docs/15)'
	}

	schema() {
		const { Type } = this.kit
		return Type.Object({
			module: this.moduleField(),
			name: this.tagField('Object name, kebab-case; becomes the Go type', ['task', 'agent-run', 'ui-request']),
			description: this.descriptionField(['Task struct + state machine + participants']),
		})
	}

	async *create(rawSpec, env) {
		const spec = this.normalize(rawSpec)
		const file = `${spec.module}/model_${snakeCase(spec.name)}.go`

		yield* this.createFresh(env, file, modelTemplate(spec))

		if (spec.intent !== undefined) {
			yield this.plan(
				spec,
				`Define the ${pascalCase(spec.name)} model object`,
				[file, `${spec.module}/model_${spec.module}.go`],
				`Intent: ${spec.intent}

Define ${pascalCase(spec.name)} in ${file} and hang it off the module's Model
slice. Plain values only: typed ids, string-typed statuses matching the test
vocabulary, copy-on-write containers, pure read helpers — no I/O, no JSON, no
locks (docs/15). Do not refactor unrelated code.`,
			)
		}
	}
}

// --- subscription --------------------------------------------------------------

class SubscriptionType extends KasiType {
	id() {
		return 'subscription'
	}

	description() {
		return 'A subscription: pure state -> set of running sources (docs/15)'
	}

	schema() {
		const { Type } = this.kit
		return Type.Object({
			module: this.moduleField(),
			name: this.tagField('Subscription name, kebab-case', ['agent-watch', 'inbox-poll']),
			emits: this.tagField('Message tag the source emits', ['finish-agent-run']),
			description: this.descriptionField(['watch harness processes; emit finish-agent-run on exit']),
		})
	}

	async *create(rawSpec, env) {
		const spec = this.normalize(rawSpec)
		const gomod = await goModulePath(env)
		const file = `${spec.module}/subscription_${snakeCase(spec.name)}.go`

		yield* this.createFresh(env, file, subscriptionTemplate(spec, gomod))
		yield* this.registerInModule(spec, env, `runtime.Subscribe(mod, ${camelCase(spec.name)}Subs)`)

		if (spec.intent !== undefined) {
			yield this.plan(
				spec,
				`Implement the ${spec.name} subscription`,
				[file],
				`Intent: ${spec.intent}

Implement ${camelCase(spec.name)}Subs in ${file}: a pure function from state
to the sources that should be running, each with a stable id. The body is an
edge-style function${spec.emits ? ` that emits "${spec.emits}"` : ''}: edges +
emit, never the model (docs/15). Do not refactor unrelated code.`,
			)
		}
	}
}

// --- view ----------------------------------------------------------------------

class ViewType extends KasiType {
	id() {
		return 'view'
	}

	description() {
		return 'An htmlc view: web/view_<name>.vue + its <Name>View struct and render helper (docs/08)'
	}

	schema() {
		const { Type } = this.kit
		return Type.Object({
			name: this.tagField('View name, kebab-case; becomes view_<name>.vue + view_<name>.go in web/', [
				'task',
				'task-list',
				'request-form',
			]),
			render: Type.Optional(
				Type.String({
					description: 'page (RenderPage, full page) or fragment (RenderFragment, a Turbo swap target) — docs/08',
					examples: ['page'],
					pattern: '^(page|fragment)$',
				}),
			),
			props: Type.Optional(
				Type.Record(Type.String({ pattern: '^[a-z][a-z0-9_]*$' }), Type.String({ description: 'Go type of the field' }), {
					description: 'Fields of the <Name>View struct the template renders',
					examples: [{ id: 'string', status: 'string', subject: 'string' }],
					cli: false,
				}),
			),
			description: this.descriptionField(['one task: thread, participants, runs, artifacts']),
		})
	}

	normalize(spec) {
		const name = spec.name

		if (name === undefined || !/^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(name)) {
			throw new this.kit.UserError(`view: name must be kebab-case, got ${JSON.stringify(name)}`)
		}

		const render = spec.render ?? 'page'

		if (!/^(page|fragment)$/.test(render)) {
			throw new this.kit.UserError(`view: render must be "page" or "fragment", got ${JSON.stringify(render)}`)
		}

		return { ...spec, name, render, module: 'web' }
	}

	async *create(rawSpec, env) {
		const spec = this.normalize(rawSpec)
		const snake = snakeCase(spec.name)
		const vueFile = `web/view_${snake}.vue`
		const goFile = `web/view_${snake}.go`

		yield* this.createFresh(env, vueFile, viewVueTemplate(spec))
		yield* this.createFresh(env, goFile, viewGoTemplate(spec))

		if (spec.intent !== undefined) {
			yield this.plan(
				spec,
				`Implement the ${spec.name} view`,
				[vueFile, goFile],
				`Intent: ${spec.intent}

Implement the ${spec.name} view. The route handler builds a ${pascalCase(spec.name)}View
from the in-RAM model and hands it to Render${pascalCase(spec.name)} — htmlc receives
map[string]any and every value in it is a View struct, never a raw model
object or an ad-hoc map (docs/08, docs/15). Semantic, mobile-first HTML that
works without JavaScript; lead with the decision or primary action. Writes go
through dispatch routes that emit runtime messages. Do not refactor unrelated
code.`,
			)
		}
	}
}

// --- form ------------------------------------------------------------------------

class FormType extends KasiType {
	id() {
		return 'form'
	}

	description() {
		return 'A form object: bind + validate + construct one runtime message; re-renders with errors (docs/08)'
	}

	schema() {
		const { Type } = this.kit
		return Type.Object({
			name: this.tagField('Form name, kebab-case; becomes web/form_<name>.go', ['allow-sender', 'update-route']),
			module: this.moduleField(),
			message: this.tagField(
				'Imperative message tag a valid submission constructs (defaults to the form name); must be in the owning module\'s contract',
				['allow-sender', 'update-route'],
			),
			fields: Type.Optional(
				Type.Record(Type.String({ pattern: '^[a-z][a-z0-9_]*$' }), Type.String({ description: 'Go type of the field' }), {
					description: 'Form fields: snake_case name -> Go type; string fields bind from the request automatically',
					examples: [{ address: 'string' }],
					cli: false,
				}),
			),
			description: this.descriptionField(['add an address to the initiator allowlist']),
		})
	}

	normalize(spec) {
		const normalized = super.normalize(spec)
		return { ...normalized, message: normalized.message ?? normalized.name }
	}

	async *create(rawSpec, env) {
		const spec = this.normalize(rawSpec)
		const gomod = await goModulePath(env)
		const file = `web/form_${snakeCase(spec.name)}.go`

		yield* this.createFresh(env, 'web/form.go', formErrorsTemplate())
		yield* this.createFresh(env, file, formTemplate(spec, gomod))

		if (spec.intent !== undefined) {
			yield this.plan(
				spec,
				`Implement the ${spec.name} form`,
				[file],
				`Intent: ${spec.intent}

Implement ${pascalCase(spec.name)}Form in ${file}: finish Bind for non-string
fields, write the Validate rules, and map the fields onto the
"${spec.message}" payload in Message. The handler loop is: bind + validate;
invalid re-renders the same view with the form (values + errors) in the props
map; valid sends the message, then redirects so the next GET renders the new
model (docs/08, docs/15). Do not refactor unrelated code.`,
			)
		}
	}
}

// --- Components ----------------------------------------------------------------

class KasiComponent {
	constructor({ kind, id, description, files, details }) {
		this.kind = kind
		this.componentID = id
		this.componentDescription = description
		this.files = files
		this.details = details
	}

	provider() {
		return 'kasi'
	}

	id() {
		return this.componentID
	}

	description() {
		return this.componentDescription
	}

	inspect() {
		return {
			name: this.componentID,
			kind: this.kind,
			...this.details,
			files: this.files,
		}
	}
}

// --- Go templates (canonical shapes: docs/15-tactical-patterns.md) -------------

function moduleTemplate(name, what, gomod) {
	return `package ${name}

import "${gomod}/runtime"

// Edges is everything ${name} touches in the world. Real implementations are
// wired in cmd/kasi/main.go; simulated twins live in this package (docs/12).
type Edges struct {
	Clock runtime.Clock
}

// Module bundles ${what} (docs/01).
func Module(e Edges) *runtime.Module {
	mod := runtime.NewModule("${name}", Model{}, e)

	return mod
}

// SimEdges is the full simulated set — what \`kasi test\` assembles by
// default, and the simulated twin the twin rule demands (docs/12).
func SimEdges() Edges {
	return Edges{
		Clock: runtime.SimClock(),
	}
}
`
}

function modelSliceTemplate(name) {
	return `package ${name}

// Model is the ${name} slice of the application model (docs/15).
type Model struct{}
`
}

function contractDocTemplate(name) {
	return `// Package msg is ${name}'s contract: the tags, payloads, and constructors other
// domains use to reach ${name}. It imports nothing but runtime (docs/15).
package msg
`
}

function messageTemplate(spec, gomod) {
	const pascal = pascalCase(spec.name)
	return `package ${spec.module}

import "${gomod}/runtime"

// "${spec.name}" — ${spec.description ?? 'TODO: one line on who sends this and what it owns'}
const ${pascal} = "${spec.name}"

${payloadStruct(pascal, spec.fields)}

func New${pascal}(p ${pascal}Payload) runtime.Msg {
	return runtime.NewMsg(${pascal}, p)
}

func register${pascal}(mod *runtime.Module) {
	runtime.HandleMsg(mod, ${pascal}, handle${pascal})
}

func handle${pascal}(v runtime.View, s Model, p ${pascal}Payload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	return s, nil
}
`
}

function contractMessageTemplate(spec, gomod) {
	const pascal = pascalCase(spec.name)
	return `package msg

import "${gomod}/runtime"

// "${spec.name}" — ${spec.description ?? 'TODO: one line on who sends this and what it owns'}
const ${pascal} = "${spec.name}"

${payloadStruct(pascal, spec.fields)}

func New${pascal}(p ${pascal}Payload) runtime.Msg {
	return runtime.NewMsg(${pascal}, p)
}
`
}

function contractHandlerTemplate(spec, gomod) {
	const pascal = pascalCase(spec.name)
	return `package ${spec.module}

import (
	"${gomod}/runtime"
	"${gomod}/${spec.module}/msg"
)

// "${spec.name}" — ${spec.description ?? 'TODO: one line on who sends this and what it owns'}

func register${pascal}(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.${pascal}, handle${pascal})
}

func handle${pascal}(v runtime.View, s Model, p msg.${pascal}Payload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	return s, nil
}
`
}

function commandTemplate(spec, gomod) {
	const pascal = pascalCase(spec.name)
	return `package ${spec.module}

import (
	"context"

	"${gomod}/runtime"
)

// "${spec.name}" — ${spec.description ?? 'TODO: one line on the effect this command describes'}
const ${pascal} = "${spec.name}"

${payloadStruct(pascal, spec.fields)}

func New${pascal}(p ${pascal}Payload) runtime.Cmd {
	return runtime.NewCmd(${pascal}, p)
}

func register${pascal}(mod *runtime.Module) {
	runtime.HandleCmd(mod, ${pascal}, ${camelCase(spec.name)}Effect)
}

func ${camelCase(spec.name)}Effect(ctx context.Context, e Edges, p ${pascal}Payload,
	emit runtime.Emit) error {
${effectBody(spec)}}
`
}

function contractCommandTemplate(spec, gomod) {
	const pascal = pascalCase(spec.name)
	return `package msg

import "${gomod}/runtime"

// "${spec.name}" — ${spec.description ?? 'TODO: one line on the effect this command describes'}
const ${pascal} = "${spec.name}"

${payloadStruct(pascal, spec.fields)}

func New${pascal}(p ${pascal}Payload) runtime.Cmd {
	return runtime.NewCmd(${pascal}, p)
}
`
}

function contractEffectTemplate(spec, gomod) {
	const pascal = pascalCase(spec.name)
	return `package ${spec.module}

import (
	"context"

	"${gomod}/runtime"
	"${gomod}/${spec.module}/msg"
)

// "${spec.name}" — ${spec.description ?? 'TODO: one line on the effect this command describes'}

func register${pascal}(mod *runtime.Module) {
	runtime.HandleCmd(mod, msg.${pascal}, ${camelCase(spec.name)}Effect)
}

func ${camelCase(spec.name)}Effect(ctx context.Context, e Edges, p msg.${pascal}Payload,
	emit runtime.Emit) error {
${effectBody(spec)}}
`
}

function effectBody(spec) {
	if (spec.emits === undefined) {
		return '\treturn nil\n'
	}
	return `\t// On success, the result enters the model as a message (docs/01):
	// emit(New${pascalCase(spec.emits)}(…))
	return nil
`
}

function modelTemplate(spec) {
	const pascal = pascalCase(spec.name)
	return `package ${spec.module}

// ${pascal} — ${spec.description ?? 'TODO: one line on this business object'} (docs/15)

type ${pascal}ID int64

type ${pascal} struct {
	ID ${pascal}ID
}
`
}

function subscriptionTemplate(spec, gomod) {
	const camel = camelCase(spec.name)
	return `package ${spec.module}

import "${gomod}/runtime"

// ${spec.name} — ${spec.description ?? 'TODO: one line on what this source watches'}
//
// A pure function from state to the set of sources that should be running,
// each with a stable id; the runtime diffs and starts/stops them (docs/01).
func ${camel}Subs(v runtime.View, s Model) []runtime.Sub {
	return nil
}
`
}

function payloadStruct(pascal, fields) {
	const body = payloadFields(fields)
	if (body === '') return `type ${pascal}Payload struct{}`
	return `type ${pascal}Payload struct {\n${body}}`
}

function payloadFields(fields) {
	const entries = Object.entries(fields ?? {})
	if (entries.length === 0) return ''

	const width = Math.max(...entries.map(([name]) => pascalCase(name).length))
	return entries
		.map(([name, goType]) => `\t${pascalCase(name).padEnd(width)} ${goType} \`json:"${name}"\`\n`)
		.join('')
}

function viewVueTemplate(spec) {
	const camel = camelCase(spec.name)
	return `<!-- view_${snakeCase(spec.name)}.vue — ${spec.description ?? 'TODO: one line on what this view shows'} (docs/08) -->
<template>
	<article class="view-${spec.name}">
		<!-- lead with the decision or the primary action; single column,
		     semantic HTML, works without JavaScript (docs/08) -->
		{{ ${camel} }}
	</article>
</template>

<style scoped>
</style>
`
}

function viewGoTemplate(spec) {
	const pascal = pascalCase(spec.name)
	const snake = snakeCase(spec.name)
	const camel = camelCase(spec.name)
	const render = spec.render === 'fragment' ? 'RenderFragment' : 'RenderPage'
	const role =
		spec.render === 'fragment'
			? 'writes the HTML fragment Turbo swaps in'
			: 'writes the full page'

	return `package web

import (
	"io"

	"github.com/dhamidi/htmlc"
)

// ${pascal}View is the data view_${snake}.vue renders — ${spec.description ?? 'TODO: one line'}.
// htmlc receives map[string]any, and idiomatically every value in it is a
// struct like this one: built from the model by the route handler, never a
// raw model object and never an ad-hoc map (docs/08, docs/15).
${viewStruct(pascal, spec.props)}

// Render${pascal} ${role} (docs/08).
func Render${pascal}(w io.Writer, engine *htmlc.Engine, view ${pascal}View) error {
	return engine.${render}(w, "view_${snake}", map[string]any{
		"${camel}": view,
	})
}
`
}

function viewStruct(pascal, props) {
	const body = viewFields(props)
	if (body === '') return `type ${pascal}View struct{}`
	return `type ${pascal}View struct {\n${body}}`
}

function viewFields(props) {
	const entries = Object.entries(props ?? {})
	if (entries.length === 0) return ''

	const width = Math.max(...entries.map(([name]) => pascalCase(name).length))
	return entries.map(([name, goType]) => `\t${pascalCase(name).padEnd(width)} ${goType}\n`).join('')
}

function formErrorsTemplate() {
	return `package web

import "flag"

// FormErrors maps a field name to its error message. Form objects carry one
// so an invalid submission re-renders the same view with values and errors
// intact; templates read it directly: v-if="form.Errors.address" (docs/08).
type FormErrors map[string]string

// Set records the first error for a field; later errors keep the first, so
// validation reads top-to-bottom and reports the primary problem per field.
func (e FormErrors) Set(field, message string) {
	if _, taken := e[field]; !taken {
		e[field] = message
	}
}

// Parse binds a raw submitted string into a rich value through the stdlib
// flag.Value interface — one string-to-value contract for forms and CLI
// flags alike (docs/15). A Set error becomes the field's error message.
func (e FormErrors) Parse(field, raw string, into flag.Value) {
	if err := into.Set(raw); err != nil {
		e.Set(field, err.Error())
	}
}
`
}

function formTemplate(spec, gomod) {
	const pascal = pascalCase(spec.name)
	const msgPascal = pascalCase(spec.message)
	const fields = formFieldList(spec)
	const rich = fields.filter((f) => f.rich)

	return `package web

import (
	"net/http"
${fields.length > 0 ? '\t"strings"\n\n' : ''}	"${gomod}/runtime"
	msg "${gomod}/${spec.module}/msg"
${richImports(rich, gomod)})

// ${pascal}Form — ${spec.description ?? 'TODO: one line on what submitting this means'}
//
// A form object carries its own values and errors: bound from the request,
// validated, re-rendered by the same view when invalid, and turned into
// exactly one imperative runtime message when valid (docs/08, docs/15). It
// is passed to htmlc as a struct value in the props map, like any View.
//
// Fields are raw strings — what the browser sent — so a re-render always
// echoes exactly what was typed. Rich values parse in Validate through the
// stdlib flag.Value contract (docs/15).
type ${pascal}Form struct {
${formStructFields(fields)}}

// Bind${pascal}Form reads the submitted values. Binding never fails — bad
// input becomes field errors in Validate, not an HTTP error.
func Bind${pascal}Form(r *http.Request) ${pascal}Form {
	return ${pascal}Form{
${formBindFields(fields)}		Errors: FormErrors{},
	}
}

// Validate returns the form with any field errors recorded.
func (f ${pascal}Form) Validate() ${pascal}Form {
${formValidateBody(rich)}	return f
}

// Valid reports whether Message may be constructed.
func (f ${pascal}Form) Valid() bool { return len(f.Errors) == 0 }

// Message constructs the one imperative message a valid submission means
// (docs/08). Call only when Valid().
func (f ${pascal}Form) Message() runtime.Msg {
${formMessageParse(rich)}	return msg.New${msgPascal}(msg.${msgPascal}Payload{
		// TODO: map the form's fields${rich.length > 0 ? ' (and parsed values)' : ''} onto the payload
	})
}
`
}

/**
 * A form field is always bound as a raw string; a non-string declared type
 * is a rich value that must implement the stdlib flag.Value interface
 * (docs/15). Unqualified rich types are assumed to live in the owning module.
 */
function formFieldList(spec) {
	return Object.entries(spec.fields ?? {}).map(([name, goType]) => {
		const rich = goType !== 'string'
		const qualified = rich && !goType.includes('.') ? `${spec.module}.${goType}` : goType
		return { name, goType: qualified, rich, pascal: pascalCase(name), camel: camelCase(name) }
	})
}

function richImports(rich, gomod) {
	const packages = [...new Set(rich.map((f) => f.goType.split('.')[0]))]
	return packages.map((pkg) => `\t"${gomod}/${pkg}"\n`).join('')
}

function formStructFields(fields) {
	const entries = [
		...fields.map((f) => [f.pascal, 'string', f.rich ? ` // parsed into ${f.goType} by Validate` : '']),
		['Errors', 'FormErrors', ''],
	]
	const width = Math.max(...entries.map(([name]) => name.length))
	return entries.map(([name, goType, note]) => `\t${name.padEnd(width)} ${goType}${note}\n`).join('')
}

function formBindFields(fields) {
	return fields.map((f) => `\t\t${f.pascal}: strings.TrimSpace(r.FormValue("${f.name}")),\n`).join('')
}

function formValidateBody(rich) {
	const parses = rich
		.map((f) => `\tvar ${f.camel} ${f.goType}\n\tf.Errors.Parse("${f.name}", f.${f.pascal}, &${f.camel})\n`)
		.join('')

	return `${parses}\t// TODO: rules the types don't already enforce, first error per field wins:
	// if f.Address == "" { f.Errors.Set("address", "an address is required") }
`
}

function formMessageParse(rich) {
	if (rich.length === 0) return ''
	return (
		rich
			.map((f) => `\tvar ${f.camel} ${f.goType}\n\t_ = ${f.camel}.Set(f.${f.pascal}) // Valid() guarantees this parses\n`)
			.join('') + '\n'
	)
}

function wireModule(source, name, gomod) {
	if (source.includes(`${name}.Module(`)) {
		return source
	}

	const withImport = source.replace(/(import \(\n)/, `$1\t"${gomod}/${name}"\n`)
	const withModule = withImport.replace(
		/(runtime\.New\(\n)/,
		`$1\t\t${name}.Module(${name}.Edges{}), // TODO: wire real edges (docs/15)\n`,
	)

	if (withModule === withImport || withImport === source) {
		return source // shape not recognised; caller reports it
	}

	return withModule
}

// --- Naming -------------------------------------------------------------------

function pascalCase(name) {
	return name
		.replace(/(?:^|[-_])([a-z0-9])/g, (_, letter) => letter.toUpperCase())
		.replace(/(Id|Url|Uri)(?=[A-Z]|$)/g, (initialism) => initialism.toUpperCase())
}

function camelCase(name) {
	const pascal = pascalCase(name)
	return pascal[0].toLowerCase() + pascal.slice(1)
}

function snakeCase(name) {
	return name.replaceAll('-', '_')
}

export default function provider(kit) {
	return new KasiProvider(kit)
}
