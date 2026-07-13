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
 * - edge          a capability a module gets from the world: one field on its Edges,
 *                 scaffolded AND wired across all five sites in one declaration (docs/12)
 * - setting       a typed, editable piece of configuration a domain contributes:
 *                 <module>/settings.go's Settings() descriptor + its flag.Value/ToForm
 *                 value type; rendered by the runtime form engine (docs/16)
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
		// Order matters for `kit generate`'s CLI type resolver: it registers only
		// the first eight yielded types as subcommands; a token past that falls
		// through to the generic `component` handler. `component` is unaffected —
		// that fallback IS its handler — so it sits at nine, and `edge` (already
		// manifest-only, never CLI-resolved) stays last. setting must be in the
		// first eight to be `kit generate kasi setting.<module>.<key>`-able.
		yield new ModuleType(this.kit)
		yield new MessageType(this.kit)
		yield new CommandType(this.kit)
		yield new ModelType(this.kit)
		yield new SubscriptionType(this.kit)
		yield new ViewType(this.kit)
		yield new FormType(this.kit)
		yield new SettingType(this.kit)
		yield new ComponentType(this.kit)
		yield new EdgeType(this.kit)
		// scenario, like edge, is manifest-/spec-only (never a CLI generate
		// subcommand — it sits past the eighth): it advertises the type discovered
		// `.test` scenarios round-trip through (kit component spec | generate --spec).
		yield new ScenarioType(this.kit)
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
				// path is the canonical, round-trippable spec field (ScenarioType's
				// schema): the t/-relative path, sans .test. The scan name dots the
				// separators; segment names use hyphens, so dots are only separators.
				details: { path: scenario.name.replaceAll('.', '/') },
			})
		}

		for (const setting of scan.settings) {
			yield new KasiComponent({
				kind: 'setting',
				id: `setting.${setting.key}`,
				description: setting.short ?? `Setting ${setting.key} (${setting.module})`,
				files: setting.files,
				details: { module: setting.module, key: setting.key },
			})
		}
	}

	async create(spec, env) {
		throw new this.kit.UserError('Use a component type (module, message, command, model, subscription, view, form, component, edge, setting) to generate')
	}
}

// --- Discovery ---------------------------------------------------------------

async function scanRepository(kit) {
	const scan = { modules: [], messages: [], commands: [], models: [], subscriptions: [], scenarios: [], views: [], forms: [], settings: [] }

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
			render = source.includes('RenderFragment') ? 'fragment' : 'page'
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

	// Each domain contributes settings from a pure `func Settings() []settings.Setting`
	// (docs/16). One ast-grep match per contributing module; the Key: (and Short:)
	// string literals inside the returned slice name each setting.
	for (const match of await astGrep(kit, 'func Settings() []settings.Setting { $$$ }')) {
		const module = moduleOf(match.file)
		for (const { key, short } of settingDescriptors(match.text)) {
			scan.settings.push({ module, key, short, files: [match.file] })
		}
	}

	return scan
}

/**
 * Pulls each setting's Key (and, when present, its Short) out of a Settings()
 * function body. Robust-but-partial: every Key: literal becomes a setting even
 * if its Short: is missing or unusual; Short is read from the slice of text
 * between this Key and the next one, so multiple settings per module resolve
 * independently.
 */
function settingDescriptors(text) {
	if (typeof text !== 'string') return []

	const keyRe = /Key:\s*"([a-z0-9_]+)"/g
	const hits = []
	let m
	while ((m = keyRe.exec(text)) !== null) {
		hits.push({ key: m[1], index: m.index })
	}

	return hits.map((hit, i) => {
		const end = i + 1 < hits.length ? hits[i + 1].index : text.length
		const block = text.slice(hit.index, end)
		return { key: hit.key, short: block.match(/Short:\s*"([^"]*)"/)?.[1] }
	})
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

	/**
	 * The public entry point kit calls. Each type implements `generate`; this
	 * wrapper runs it and then gofmt's the Go files it actually wrote, so
	 * generated code is gofmt-clean the moment it lands (kit's templates are
	 * canonical in shape, not whitespace). Files that were consulted but left
	 * untouched (createFresh's fileRead) are NOT reformatted.
	 */
	async *create(rawSpec, env) {
		yield* this.gofmtWritten(env, this.generate(rawSpec, env))
	}

	/**
	 * Passes an inner generator's events through unchanged, collecting the Go
	 * files it created or edited, then formats exactly those with gofmt. Uses
	 * env.exec so it no-ops in dry-run (kit generate -n, manifest plan) — the
	 * files were not written, so there is nothing to format.
	 */
	async *gofmtWritten(env, inner) {
		const written = []
		for await (const event of inner) {
			yield event
			const json = typeof event?.toJSON === 'function' ? event.toJSON() : event
			if (
				json &&
				(json.type === 'file.created' || json.type === 'file.edited') &&
				typeof json.path === 'string' &&
				json.path.endsWith('.go')
			) {
				written.push(json.path.replace(/^file:\/\//, ''))
			}
		}

		if (written.length === 0) {
			return
		}

		const result = await env.exec(['gofmt', '-w', ...new Set(written)])
		if (result.code !== 0) {
			yield this.kit.Event.error(`gofmt failed (exit ${result.code}): ${result.stderr.trim()}`)
		}
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
	 * Wires a new module into every assembly list (docs/01: main.go is THE
	 * assembly point; BUILDING rule 4: forgetting to wire is the silent
	 * divergence this provider exists to prevent). The project keeps its module
	 * lists under cmd/kasi — main.go's serve+test `assembly(sim bool)` (two
	 * branches) and the test runner's sim-world `assembleSim` — so this wires
	 * each file there that builds a `[]*runtime.Module{ … }`, inserting a
	 * compiling placeholder the implementer fills with real edges. Before stage
	 * zero exists there is no such file and that is fine.
	 */
	async *wireIntoAssembly(spec, env, gomod) {
		const files = []
		for await (const path of new Glob('cmd/kasi/*.go').scan({ cwd: process.cwd() })) {
			files.push(path)
		}
		files.sort()

		if (files.length === 0) {
			return // no assembly yet — nothing to wire
		}

		let sawAssembly = false
		for (const path of files) {
			const source = await env.readFile(path)
			if (!source.includes('[]*runtime.Module{')) {
				continue // not an assembly file
			}
			sawAssembly = true

			const wired = wireModuleIntoFile(source, spec.name, gomod)
			if (wired === source) {
				if (source.includes(`${spec.name}.Module(`)) {
					yield this.kit.Event.fileRead(path) // already wired
				} else {
					yield this.kit.Event.error(
						`could not wire ${spec.name} into ${path}: no []*runtime.Module{ list to extend (docs/01)`,
					)
				}
				continue
			}

			yield await env.editFile(path, () => wired)
		}

		if (!sawAssembly) {
			yield this.kit.Event.error(
				`could not wire ${spec.name}: no []*runtime.Module{ assembly list found under cmd/kasi/ (docs/01)`,
			)
		}
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

	async *generate(rawSpec, env) {
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

	async *generate(rawSpec, env) {
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

	async *generate(rawSpec, env) {
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

	async *generate(rawSpec, env) {
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

	async *generate(rawSpec, env) {
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

	async *generate(rawSpec, env) {
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

	async *generate(rawSpec, env) {
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

// --- component -------------------------------------------------------------------

class ComponentType extends KasiType {
	id() {
		return 'component'
	}

	description() {
		return 'A reusable htmlc sub-component: a plain SFC fragment a page composes by its <name> tag — no .go render helper, no page wrapper (docs/08)'
	}

	schema() {
		const { Type } = this.kit
		return Type.Object({
			name: this.tagField('Component name, kebab-case; becomes web/<snake>.vue and the <snake> custom-element tag the parent page uses', [
				'request-field',
				'request-summary',
				'task-card',
			]),
			props: Type.Optional(
				Type.Record(Type.String({ pattern: '^[a-z][a-zA-Z0-9]*$' }), Type.String({ description: 'Example of what the parent binds (a View type or literal)' }), {
					description: 'Props the parent page binds (<tag :prop="…">): prop name -> example value, documented in the SFC header comment',
					examples: [{ field: 'FieldView', message: 'string' }],
					cli: false,
				}),
			),
			description: this.descriptionField(['one spec field: label + a control chosen by type + error']),
		})
	}

	normalize(spec) {
		const name = spec.name

		if (name === undefined || !/^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(name)) {
			throw new this.kit.UserError(`component: name must be kebab-case, got ${JSON.stringify(name)}`)
		}

		return { ...spec, name, module: 'web' }
	}

	async *generate(rawSpec, env) {
		const spec = this.normalize(rawSpec)
		const vueFile = `web/${snakeCase(spec.name)}.vue`

		yield* this.createFresh(env, vueFile, componentVueTemplate(spec))

		if (spec.intent !== undefined) {
			yield this.plan(
				spec,
				`Implement the ${spec.name} component`,
				[vueFile],
				`Intent: ${spec.intent}

Implement the ${spec.name} sub-component in ${vueFile}. It is a plain SFC
fragment — a single-rooted <template> plus <style scoped>, no <html> wrapper
and no .go render helper. The parent page composes it with the <${snakeCase(spec.name)}>
custom-element tag htmlc derives from the filename, binding each prop as
<${snakeCase(spec.name)} :prop="…">. Semantic, mobile-first HTML that works
without JavaScript (docs/08). Do not refactor unrelated code.`,
			)
		}
	}
}

// --- edge --------------------------------------------------------------------

class EdgeType extends KasiType {
	id() {
		return 'edge'
	}

	description() {
		return 'A module edge: one field on a module\'s Edges struct, scaffolded AND wired across all five sites (module.go real+sim twins, serve.go, simworld.go) in one declaration (docs/12)'
	}

	schema() {
		const { Type } = this.kit
		return Type.Object({
			module: Type.Optional(
				Type.String({
					description: 'Domain module (Go package) whose Edges struct gains this field',
					examples: ['agents', 'tasks', 'email'],
					pattern: '^[a-z][a-z0-9]*$',
					cli: false,
				}),
			),
			name: Type.Optional(
				Type.String({
					description: 'PascalCase field name on the Edges struct',
					examples: ['Store', 'Content', 'Mail'],
					pattern: '^[A-Z][A-Za-z0-9]*$',
					cli: false,
				}),
			),
			type: Type.Optional(
				Type.String({
					description: 'Go type of the edge field; the import is derived from its leading package identifier',
					examples: ['datastore.Store', 'store.Content', 'runtime.Clock'],
					cli: false,
				}),
			),
			sim: Type.Optional(
				Type.String({
					description: 'Sim-twin expression wired into SimEdges (and the shared sim world when shared)',
					examples: ['datastore.NewSim()', 'store.NewMemoryContent()', 'runtime.SimClock()'],
					cli: false,
				}),
			),
			real: Type.Optional(
				Type.String({
					description: 'Expression wired at the serve.go assembly site; its identifiers must already exist there (the human wires the real value)',
					examples: ['dataStore', 'content', 'clock'],
					cli: false,
				}),
			),
			shared: Type.Optional(
				Type.Boolean({
					description: 'true → the edge is shared across modules: also add a simWorld struct field + wire it in newSimWorld/newRecordedWorld/newLiveWorld, and reference w.<field> in assembleSim',
					examples: [true],
				}),
			),
			description: this.descriptionField(["the agent's persistent data store (Flow F)"]),
		})
	}

	normalize(rawSpec) {
		const module = rawSpec.module ?? rawSpec.parent
		const name = rawSpec.name

		if (module === undefined || !/^[a-z][a-z0-9]*$/.test(module)) {
			throw new this.kit.UserError(`edge: module must be a lower-case Go package name, got ${JSON.stringify(module)}`)
		}
		if (RESERVED_DIRS.has(module)) {
			throw new this.kit.UserError(`edge: ${module} is not a domain module`)
		}
		if (name === undefined || !/^[A-Z][A-Za-z0-9]*$/.test(name)) {
			throw new this.kit.UserError(`edge: name must be a PascalCase Go field name, got ${JSON.stringify(name)}`)
		}
		if (rawSpec.type === undefined || rawSpec.type === '') {
			throw new this.kit.UserError('edge: type is required (the Go type of the field, e.g. datastore.Store)')
		}
		if (rawSpec.sim === undefined || rawSpec.sim === '') {
			throw new this.kit.UserError('edge: sim is required (the sim-twin expression, e.g. "datastore.NewSim()")')
		}
		if (rawSpec.real === undefined || rawSpec.real === '') {
			throw new this.kit.UserError('edge: real is required (the expression at the serve.go assembly site, e.g. "dataStore")')
		}

		return { ...rawSpec, module, name, shared: rawSpec.shared ?? false, field: camelCase(name) }
	}

	async *generate(rawSpec, env) {
		const spec = this.normalize(rawSpec)
		const gomod = await goModulePath(env)
		const typePkg = leadingPackage(spec.type)
		const simPkg = leadingPackage(spec.sim)

		// Sites 1 + 2: <module>/module.go — the Edges struct field (with its
		// description as a comment) and the SimEdges twin entry, plus imports.
		yield* this.editModuleGo(spec, env, gomod, typePkg, simPkg)

		// Sites 3 + 4 (+ 5): the assembly files under cmd/kasi — serve.go (REAL
		// value) and simworld.go's assembleSim (the sim world; + the simWorld
		// struct field and its constructors when the edge is shared).
		yield* this.editAssembly(spec, env, gomod, typePkg, simPkg)

		if (spec.intent !== undefined) {
			yield this.plan(
				spec,
				`Implement the ${spec.module}.${spec.name} edge`,
				[`${spec.module}/module.go`, 'cmd/kasi/serve.go', 'cmd/kasi/simworld.go'],
				`Intent: ${spec.intent}

The ${spec.name} edge is wired across all five sites (docs/12). Define the
${spec.type} contract and its sim twin (${spec.sim}); make the real value
(${spec.real}) exist at the serve.go assembly site. Do not refactor unrelated
code.`,
			)
		}
	}

	async *editModuleGo(spec, env, gomod, typePkg, simPkg) {
		const modulePath = `${spec.module}/module.go`

		if (!(await Bun.file(modulePath).exists())) {
			if (env.dryRun) {
				yield this.kit.Event.fileEdited(modulePath)
				return
			}
			yield this.kit.Event.error(
				`${modulePath} does not exist; generate the module first: kit generate kasi module.${spec.module}`,
			)
			return
		}

		yield await env.editFile(modulePath, (source) => {
			let s = source
			s = addStructField(s, 'Edges', spec.name, spec.type, spec.description).source
			s = addFieldToLiterals(s, 'return Edges{', spec.name, spec.sim).source
			s = ensureImport(s, importPath(gomod, typePkg))
			s = ensureImport(s, importPath(gomod, simPkg))
			return s
		})
	}

	async *editAssembly(spec, env, gomod, typePkg, simPkg) {
		const anchor = `${spec.module}.Module(${spec.module}.Edges{`

		const files = []
		for await (const path of new Glob('cmd/kasi/*.go').scan({ cwd: process.cwd() })) {
			files.push(path)
		}
		files.sort()

		let sawReal = false
		let sawSim = false
		for (const path of files) {
			const source = await env.readFile(path)
			if (!source.includes(anchor)) {
				continue
			}

			if (source.includes('func assembleSim(')) {
				sawSim = true
				yield await env.editFile(path, (s) => this.wireSimFile(s, spec, gomod, typePkg, simPkg, anchor))
			} else {
				sawReal = true
				yield await env.editFile(path, (s) => addFieldToLiterals(s, anchor, spec.name, spec.real).source)
			}
		}

		if (!sawReal) {
			yield this.kit.Event.error(
				`could not wire ${spec.module}.${spec.name}: no real assembly site (${anchor}…) found under cmd/kasi/ (docs/12)`,
			)
		}
		if (!sawSim) {
			yield this.kit.Event.error(
				`could not wire ${spec.module}.${spec.name}: no assembleSim site (${anchor}…) found under cmd/kasi/ (docs/12)`,
			)
		}
	}

	/**
	 * Wires the edge into the sim world file. assembleSim references w.<field>
	 * for a SHARED edge (the value lives on the shared simWorld, so email/tasks
	 * see the same instance) or the sim expression directly otherwise. A shared
	 * edge also adds the field to the simWorld struct and sets it in every
	 * constructor (newSimWorld/newRecordedWorld/newLiveWorld).
	 */
	wireSimFile(source, spec, gomod, typePkg, simPkg, anchor) {
		let s = source
		const assemblyExpr = spec.shared ? `w.${spec.field}` : spec.sim
		s = addFieldToLiterals(s, anchor, spec.name, assemblyExpr).source

		if (spec.shared) {
			s = addStructField(s, 'simWorld', spec.field, spec.type, undefined).source
			s = addFieldToLiterals(s, '&simWorld{', spec.field, spec.sim).source
			s = ensureImport(s, importPath(gomod, typePkg))
		}
		s = ensureImport(s, importPath(gomod, simPkg))
		return s
	}
}

// --- setting -----------------------------------------------------------------

class SettingType extends KasiType {
	id() {
		return 'setting'
	}

	description() {
		return 'A typed, editable setting a domain contributes: <module>/settings.go\'s Settings() descriptor + its flag.Value/ToForm value type, rendered by the runtime form engine (docs/16)'
	}

	schema() {
		const { Type } = this.kit
		return Type.Object({
			module: this.moduleField(),
			key: Type.Optional(
				Type.String({
					description: 'Setting key: a stable snake_case (or kebab-case) id — the route param and the form\'s scope (docs/16)',
					examples: ['base_url', 'reply_from', 'max_task_runs'],
					pattern: '^[a-z][a-z0-9]*(?:[-_][a-z0-9]+)*$',
					cli: false,
				}),
			),
			short: Type.Optional(
				Type.String({
					description: 'One line, shown in the settings list',
					examples: ['Reply-from address', 'Public base URL for capability links'],
				}),
			),
			long: Type.Optional(
				Type.String({
					description: 'Help text, shown on the setting\'s form',
					examples: ['The deliverable From address käsi sends replies as (docs/04).'],
				}),
			),
			value_type: Type.Optional(
				Type.String({
					description: 'Go type name of the value — a leaf implementing flag.Value (Set/String) + ToForm; defaults to the PascalCase key',
					examples: ['MaxConcurrent', 'BaseURL', 'FromAddress'],
					pattern: '^[A-Z][A-Za-z0-9]*$',
				}),
			),
			message: Type.Optional(
				this.tagField('The set-* message tag a write emits (defaults to set-<key>)', ['set-base-url', 'set-max-concurrent-runs']),
			),
		})
	}

	normalize(rawSpec) {
		const module = rawSpec.module ?? rawSpec.parent
		const rawKey = rawSpec.key ?? rawSpec.name

		if (module === undefined || !/^[a-z][a-z0-9]*$/.test(module)) {
			throw new this.kit.UserError(`setting: module must be a lower-case Go package name, got ${JSON.stringify(module)}`)
		}
		if (RESERVED_DIRS.has(module)) {
			throw new this.kit.UserError(`setting: ${module} is not a domain module`)
		}
		if (rawKey === undefined || !/^[a-z][a-z0-9]*(?:[-_][a-z0-9]+)*$/.test(rawKey)) {
			throw new this.kit.UserError(`setting: key must be snake_case or kebab-case, got ${JSON.stringify(rawKey)}`)
		}

		const key = rawKey.replaceAll('-', '_')
		const valueType = rawSpec.value_type ?? pascalCase(key)
		const short = rawSpec.short ?? `The ${key.replaceAll('_', ' ')} setting`
		const long = rawSpec.long ?? short
		const message = rawSpec.message ?? `set-${key.replaceAll('_', '-')}`

		return { ...rawSpec, module, key, name: key, valueType, short, long, message }
	}

	async *generate(rawSpec, env) {
		const spec = this.normalize(rawSpec)
		const gomod = await goModulePath(env)
		const settingsFile = `${spec.module}/settings.go`

		// A fresh module: write the whole contribution — a Settings() returning the
		// one descriptor plus the value-type skeleton — deterministically (~80%).
		if (!(await Bun.file(settingsFile).exists())) {
			yield* this.createFresh(env, settingsFile, settingsFileTemplate(spec, gomod))

			if (spec.intent !== undefined) {
				yield this.plan(
					spec,
					`Implement the "${spec.key}" setting`,
					[settingsFile],
					newSettingPrompt(spec),
				)
			}
			return
		}

		// settings.go already exists: splicing into the returned slice literal is
		// brittle, so we don't. Emit the value-type skeleton as its own file (if the
		// type is new) and hand the descriptor addition to the implementer as a plan.
		const settingsSource = await env.readFile(settingsFile)
		const valueFile = `${spec.module}/setting_${spec.key}.go`

		if (!new RegExp(`type ${escapeRegExp(spec.valueType)}\\b`).test(settingsSource)) {
			yield* this.createFresh(env, valueFile, settingValueTemplate(spec, gomod))
		}

		yield this.plan(
			spec,
			`Add the "${spec.key}" setting to ${spec.module}.Settings()`,
			[settingsFile, valueFile],
			existingSettingPrompt(spec),
		)
	}
}

// --- scenario ------------------------------------------------------------------

// ScenarioType advertises the type that discovered `t/**/*.test` scenarios round-
// trip through (docs/13, docs/14). A scenario has no module or tag — its identity
// is its path under t/, so `path` (ring/name, sans .test) is the whole canonical
// spec, matching what KasiComponent.inspect() reports for a discovered scenario.
class ScenarioType extends KasiType {
	id() {
		return 'scenario'
	}

	description() {
		return 'A `kasi test` scenario: a t/<ring>/<name>.test script driven by the test runner, asserting rendered markers and model/edge state — never *_test.go (docs/13, docs/14)'
	}

	schema() {
		const { Type } = this.kit
		return Type.Object({
			path: Type.String({
				description: 'Scenario path under t/, without the .test suffix (ring/name)',
				examples: ['research/memory-forget', 'web/skills'],
				pattern: '^[a-z0-9]+(?:-[a-z0-9]+)*(?:/[a-z0-9]+(?:-[a-z0-9]+)*)+$',
			}),
		})
	}

	// A scenario carries no module/tag, so the base normalize() doesn't fit. Accept
	// an explicit `path`, the CLI/manifest `scenario.<ring>.<name>` (parent+name)
	// form, or a `ring`+`name` pair — all spell the same t/-relative path.
	normalize(spec) {
		let path = spec.path
		if (path === undefined) {
			const ring = spec.ring ?? spec.parent
			const name = spec.name
			if (ring !== undefined && name !== undefined) {
				path = `${ring}/${name}`
			}
		}
		if (typeof path !== 'string' || !/^[a-z0-9]+(?:-[a-z0-9]+)*(?:\/[a-z0-9]+(?:-[a-z0-9]+)*)+$/.test(path)) {
			throw new this.kit.UserError(`${this.id()}: path must be a t/-relative scenario path like "research/memory-forget", got ${JSON.stringify(path)}`)
		}
		return { ...spec, path }
	}

	async *generate(rawSpec, env) {
		const spec = this.normalize(rawSpec)
		const file = `t/${spec.path}.test`

		yield* this.createFresh(env, file, scenarioTemplate(spec))

		if (spec.intent !== undefined) {
			yield this.plan(
				spec,
				`Write the ${spec.path} scenario`,
				[file],
				`Intent: ${spec.intent}

Write ${file} as a Tcl-style scenario script (docs/14): drive the domain with the
deliver/agent/visit/post/model/task vocabulary and assert on real rendered markers
and model/edge state, never internals. No *_test.go — the scenario IS the test
(docs/13). Do not refactor unrelated code.`,
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

	// type names the advertised component type this instance was discovered as —
	// the exact key kit's `provider test`/`component spec` match on (a component's
	// kind IS its type id here), so matching never falls back to fuzzy name-guessing.
	type() {
		return this.kind
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

function scenarioTemplate(spec) {
	const what = spec.description ?? `the ${spec.path} scenario`
	return `# t/${spec.path}.test — ${what}
#
# A Tcl-style scenario script (docs/14): drive the domain and assert on rendered
# markers and model/edge state. Replace this skeleton with the real steps.

`
}

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

// settingValueTemplate is the setting's value type: a flag.Value leaf (Set +
// String) that forms through the default former (settings.FormOf). A string-based
// skeleton compiles as-is; the implementer refines Set (parse-don't-validate) and
// switches to an int/struct base where the setting calls for one (docs/16).
function settingValueTemplate(spec, gomod) {
	const vt = spec.valueType
	return `package ${spec.module}

import "${gomod}/settings"

// ${vt} is the typed value of the "${spec.key}" setting — ${spec.short}.
// It parses through flag.Value (Set/String) and forms through the default former
// (settings.FormOf, one field). The value's state stays in ${spec.module}.Model;
// this type is the read/write shape over it (docs/16).
type ${vt} string

func (v *${vt}) Set(raw string) error {
	// TODO: parse-don't-validate — reject anything ${vt} must not hold (docs/15).
	*v = ${vt}(raw)
	return nil
}

func (v ${vt}) String() string          { return string(v) }
func (v ${vt}) ToForm() settings.Form { return settings.FormOf(&v) }
`
}

// settingsFileTemplate is the whole <module>/settings.go for a module that has no
// settings yet: the value-type skeleton plus a Settings() returning its one
// descriptor. Read/Write are TODO closures that still compile (Read returns the
// zero value, Write the zero Msg), so the file is gofmt-clean and buildable while
// the implementer fills them in (docs/16, decision-020).
function settingsFileTemplate(spec, gomod) {
	const vt = spec.valueType
	return `package ${spec.module}

import (
	"${gomod}/runtime"
	"${gomod}/settings"
)

// ${vt} is the typed value of the "${spec.key}" setting — ${spec.short}.
// It parses through flag.Value (Set/String) and forms through the default former
// (settings.FormOf, one field). The value's state stays in ${spec.module}.Model;
// this type is the read/write shape over it (docs/16).
type ${vt} string

func (v *${vt}) Set(raw string) error {
	// TODO: parse-don't-validate — reject anything ${vt} must not hold (docs/15).
	*v = ${vt}(raw)
	return nil
}

func (v ${vt}) String() string          { return string(v) }
func (v ${vt}) ToForm() settings.Form { return settings.FormOf(&v) }

// Settings is ${spec.module}'s contribution to the settings surface (docs/16,
// decision-020). A pure function main.go concatenates into web.Settings(...); no
// registry, no init(). The value stays in ${spec.module}'s own model — this is a
// read plus a write over it, not a relocation.
func Settings() []settings.Setting {
	return []settings.Setting{{
		Key:   "${spec.key}",
		Short: "${spec.short}",
		Long:  "${spec.long}",
		Owner: "${spec.module}",
		Read: func(v runtime.View) settings.Value {
			// TODO: read the current value out of the model through a pure View
			// read helper (docs/15), wrapped in ${vt}.
			return ${vt}("")
		},
		Write: func(val settings.Value) runtime.Msg {
			// TODO: build the "${spec.message}" message this setting writes, e.g.
			// msg.New${pascalCase(spec.message)}(...); its payload carries val.(${vt}).
			return runtime.Msg{}
		},
	}}
}
`
}

function newSettingPrompt(spec) {
	return `Intent: ${spec.intent ?? `the ${spec.key} setting for ${spec.module}`}

Finish ${spec.module}/settings.go. Make ${spec.valueType}.Set enforce what the
value may hold (parse-don't-validate, docs/15), switching to an int/struct base
if a string is wrong. Fill the descriptor's Read to pull the live value out of
${spec.module}.Model through a pure View helper, and Write to build the
"${spec.message}" set-* message (add the message + its payload if it does not
exist yet). Wire ${spec.module}.Settings() into web.Settings(...) in
cmd/kasi/serve.go beside the other modules (docs/16). Do not refactor unrelated
code.`
}

function existingSettingPrompt(spec) {
	return `Intent: ${spec.intent ?? `add the ${spec.key} setting to ${spec.module}`}

${spec.module}/settings.go already has a Settings() slice; add one more
settings.Setting descriptor to it (do not rewrite the others):

	{
		Key:   "${spec.key}",
		Short: "${spec.short}",
		Long:  "${spec.long}",
		Owner: "${spec.module}",
		Read:  func(v runtime.View) settings.Value { return ${spec.valueType}(/* model field */) },
		Write: func(val settings.Value) runtime.Msg { return msg.New${pascalCase(spec.message)}(/* payload from val.(${spec.valueType}) */) },
	}

The ${spec.valueType} value type is scaffolded in
${spec.module}/setting_${spec.key}.go — refine its Set (parse-don't-validate,
docs/15). Read wraps the live model field in ${spec.valueType}; Write builds the
"${spec.message}" message (add it + its payload to ${spec.module}/msg if it does
not exist). Do not refactor unrelated code.`
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

function componentVueTemplate(spec) {
	const snake = snakeCase(spec.name)
	const kebab = spec.name
	const props = Object.keys(spec.props ?? {})

	const propsDoc =
		props.length > 0
			? `\n     Props the parent binds (<${snake} ${props.map((p) => `:${p}="…"`).join(' ')}>):\n${props
					.map((p) => `       ${p} — ${spec.props[p]}`)
					.join('\n')}`
			: `\n     Props: none yet — the parent binds them as <${snake} :prop="…">.`

	const body =
		props.length > 0
			? props.map((p) => `\t\t{{ ${p} }}`).join('\n')
			: `\t\t<!-- reference props as {{ prop }}, loop with v-for, branch with v-if (like request_field.vue) -->`

	return `<!-- ${snake}.vue — ${spec.description ?? 'TODO: one line on what this fragment shows'}.
     A reusable htmlc sub-component: a plain SFC fragment (a single-rooted
     <template> + <style scoped>), no <html> wrapper and no .go render helper.
     The parent page composes it by the <${snake}> custom-element tag htmlc
     derives from this filename (docs/08).${propsDoc} -->
<template>
	<div class="${kebab}">
${body}
	</div>
</template>

<style scoped>
.${kebab} {
}
</style>
`
}

function viewGoTemplate(spec) {
	const pascal = pascalCase(spec.name)
	const snake = snakeCase(spec.name)
	const camel = camelCase(spec.name)
	const render = spec.render === 'fragment' ? 'RenderFragmentContext' : 'RenderPage'
	const role =
		spec.render === 'fragment'
			? 'writes the HTML fragment Turbo swaps in'
			: 'writes the full page'

	return `package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"
)

// ${pascal}View is the data view_${snake}.vue renders — ${spec.description ?? 'TODO: one line'}.
// htmlc receives map[string]any, and idiomatically every value in it is a
// struct like this one: built from the model by the route handler, never a
// raw model object and never an ad-hoc map (docs/08, docs/15).
${viewStruct(pascal, spec.props)}

// Render${pascal} ${role} (docs/08).
func Render${pascal}(ctx context.Context, w io.Writer, engine *htmlc.Engine, view ${pascal}View) error {
	return engine.${render}(ctx, w, "view_${snake}", map[string]any{
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

// wireModuleIntoFile names a module in one assembly file: it adds the import (if
// missing) and inserts a placeholder entry into EVERY `[]*runtime.Module{ … }`
// literal in the file — both branches of `assembly(sim bool)`, or the sim
// world's list. The entry is a compiling stub with an empty Edges and a TODO;
// the implementer fills in the real edges (docs/15). Idempotent per file: a file
// that already names the module is left untouched.
function wireModuleIntoFile(source, name, gomod) {
	if (source.includes(`${name}.Module(`)) {
		return source
	}

	let next = source
	if (!next.includes(`"${gomod}/${name}"`)) {
		next = next.replace(/(import \(\n)/, `$1\t"${gomod}/${name}"\n`)
	}

	next = next.replace(
		/(\[\]\*runtime\.Module\{\n)/g,
		`$1\t\t${name}.Module(${name}.Edges{}), // TODO: wire edges (docs/15)\n`,
	)

	if (next === source) {
		return source // no import block and no module list recognised; caller reports it
	}

	return next
}

// --- Go-source surgery (edge wiring) ------------------------------------------

/**
 * The package identifier a qualified type/expression reaches for — the leading
 * `pkg` of `pkg.Thing` or `pkg.Ctor()`. An unqualified type (a type local to
 * the module) has none, so no import is derived.
 */
function leadingPackage(expr) {
	return expr.match(/^\s*([a-z][A-Za-z0-9_]*)\s*\./)?.[1]
}

/** The Go import path for a package under this repo's module (docs/01). */
function importPath(gomod, pkg) {
	return pkg === undefined ? undefined : `${gomod}/${pkg}`
}

function escapeRegExp(text) {
	return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

/**
 * Adds an import to a Go file's import block, idempotently. Handles the two
 * shapes the generated code uses: a factored `import ( … )` block (append into
 * it) and a single `import "x"` line (promote to a block). gofmt (run by the
 * create wrapper) sorts the result, so insertion order does not matter.
 */
function ensureImport(source, path) {
	if (path === undefined) return source
	if (new RegExp(`"${escapeRegExp(path)}"`).test(source)) return source

	if (/import \(\n/.test(source)) {
		return source.replace(/import \(\n/, `import (\n\t"${path}"\n`)
	}
	const single = source.match(/import "([^"]+)"\n/)
	if (single) {
		return source.replace(/import "([^"]+)"\n/, `import (\n\t"$1"\n\t"${path}"\n)\n`)
	}
	return source.replace(/^(package \w+\n)/, `$1\nimport "${path}"\n`)
}

/**
 * Adds `<fieldName> <goType>` (preceded by <description> as a // comment) inside
 * `type <structName> struct { … }`, just after the opening brace. Idempotent: a
 * struct that already declares the field is left untouched.
 */
function addStructField(source, structName, fieldName, goType, description) {
	const openRe = new RegExp(`(type ${escapeRegExp(structName)} struct \\{\\n)`)
	const open = source.match(openRe)
	if (!open) return { source, found: false, changed: false }

	const blockRe = new RegExp(`type ${escapeRegExp(structName)} struct \\{\\n([\\s\\S]*?)\\n\\}`)
	const block = source.match(blockRe)
	if (block && new RegExp(`(^|\\n)\\s*${escapeRegExp(fieldName)}\\s`).test(block[1])) {
		return { source, found: true, changed: false }
	}

	const comment = description ? `\t// ${description}\n` : ''
	return { source: source.replace(openRe, `$1${comment}\t${fieldName} ${goType}\n`), found: true, changed: true }
}

/**
 * Adds `<name>: <expr>` to EVERY composite literal opened by <anchor> (a literal
 * string ending in `{`), just after the brace — so `&simWorld{` hits all three
 * constructors while a single assembly call hits its one literal. Idempotent per
 * literal: one that already sets the key is skipped. Works for both a one-line
 * literal (`Edges{Clock: c}`) and a multi-line one (`Edges{\n\tClock: c,\n}`);
 * gofmt tidies the whitespace afterward.
 */
function addFieldToLiterals(source, anchor, name, expr) {
	let changed = false
	const starts = []
	for (let i = source.indexOf(anchor); i !== -1; i = source.indexOf(anchor, i + anchor.length)) {
		starts.push(i)
	}

	for (const start of starts.reverse()) {
		const brace = start + anchor.length - 1
		const close = matchBrace(source, brace)
		if (close === -1) continue

		const body = source.slice(brace + 1, close)
		if (new RegExp(`(^|[\\s{,])${escapeRegExp(name)}:`).test(body)) continue

		const insertion = source[brace + 1] === '\n' ? `\n\t${name}: ${expr},` : `${name}: ${expr}, `
		source = source.slice(0, brace + 1) + insertion + source.slice(brace + 1)
		changed = true
	}

	return { source, changed, found: starts.length > 0 }
}

/** Index of the `}` that closes the `{` at openIndex (brace-count; -1 if none). */
function matchBrace(source, openIndex) {
	let depth = 0
	for (let i = openIndex; i < source.length; i++) {
		if (source[i] === '{') depth++
		else if (source[i] === '}' && --depth === 0) return i
	}
	return -1
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
