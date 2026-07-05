import { Glob } from 'bun'

/**
 * Kit provider for käsi's domain primitives (docs/15-tactical-patterns.md):
 *
 * - module        a domain package: module.go + model slice + msg/ seam
 * - message       message_<tag>.go: tag const + payload + handler + registration
 * - command       command_<tag>.go: tag const + payload + constructor + effect
 * - model         model_<name>.go: a model slice / business object
 * - subscription  subscription_<name>.go: state -> set of running sources
 *
 * Discovery is structural (ast-grep over Go source); generation emits the
 * canonical shapes from the pattern book, so generated code and documented
 * code are the same code.
 */

const RUNTIME_PACKAGE = 'runtime'
const RESERVED_DIRS = new Set(['runtime', 'cmd', 'testlang', 't', 'providers', 'docs', 'vendor'])

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
					seam: message.seam,
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
					seam: command.seam,
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
	}

	async create(spec, env) {
		throw new this.kit.UserError('Use a component type (module, message, command, model, subscription) to generate')
	}
}

// --- Discovery ---------------------------------------------------------------

async function scanRepository(kit) {
	const scan = { modules: [], messages: [], commands: [], models: [], subscriptions: [] }
	const goFiles = await glob('*/**.go')

	if (goFiles.length === 0) {
		return scan
	}

	for await (const path of new Glob('*/module.go').scan({ cwd: process.cwd() })) {
		const name = path.split('/')[0]
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
			seam: resolved.file.includes('/msg/'),
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
		return 'A käsi domain module: module.go + model slice + msg/ seam package (docs/15)'
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

		yield await env.createFile(`${spec.name}/module.go`, moduleTemplate(spec.name, what, gomod))
		yield await env.createFile(`${spec.name}/model_${spec.name}.go`, modelSliceTemplate(spec.name))
		yield await env.createFile(`${spec.name}/msg/doc.go`, seamDocTemplate(spec.name))

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
			seam: Type.Optional(
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
		const gomod = await goModulePath(env)
		const snake = snakeCase(spec.name)
		const messageFile = `${spec.module}/message_${snake}.go`
		const files = [messageFile]

		if (spec.seam) {
			const seamFile = `${spec.module}/msg/${snake}.go`
			yield await env.createFile(seamFile, seamMessageTemplate(spec, gomod))
			yield await env.createFile(messageFile, seamHandlerTemplate(spec, gomod))
			files.push(seamFile)
		} else {
			yield await env.createFile(messageFile, messageTemplate(spec, gomod))
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
			seam: Type.Optional(
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
		const gomod = await goModulePath(env)
		const snake = snakeCase(spec.name)
		const commandFile = `${spec.module}/command_${snake}.go`
		const files = [commandFile]

		if (spec.seam) {
			const seamFile = `${spec.module}/msg/${snake}.go`
			yield await env.createFile(seamFile, seamCommandTemplate(spec, gomod))
			yield await env.createFile(commandFile, seamEffectTemplate(spec, gomod))
			files.push(seamFile)
		} else {
			yield await env.createFile(commandFile, commandTemplate(spec, gomod))
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

		yield await env.createFile(file, modelTemplate(spec))

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

		yield await env.createFile(file, subscriptionTemplate(spec, gomod))
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
func Module(e Edges) runtime.Module {
	mod := runtime.NewModule("${name}", Model{}, e)

	return mod
}

// SimEdges is the full simulated set — what \`kasi test\` assembles by
// default, and the twin the seam rule demands (docs/12).
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

function seamDocTemplate(name) {
	return `// Package msg is ${name}'s seam: the tags, payloads, and constructors other
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

function seamMessageTemplate(spec, gomod) {
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

function seamHandlerTemplate(spec, gomod) {
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

function seamCommandTemplate(spec, gomod) {
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

function seamEffectTemplate(spec, gomod) {
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
