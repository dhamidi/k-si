# Kit Manifests

A manifest is a reviewable declaration of components that should exist. It is
declarative: say **what** components exist, not **how** files are edited.

Three layers (see `LANGUAGE.md` in a Kit checkout for the full spec):

```txt
TCL-ish manifest ──parse──▶ IR { provider, type, spec } ──execute──▶ type.create(spec, env)
```

The surface syntax is replaceable; the **IR and provider TypeBox schemas are the
contract**.

## Surface syntax (preferred: TCL-like)

Each top-level form is `<provider> <type> { <schema-directed fields> }`. The head
must resolve to a discovered provider name and one of its type ids.

```kit
kit-command component {
	name greet
	description "Say hello to someone"
	arg who
}

kit-event event {
	family provider
	name providerLoaded
}
```

Multiple forms per file are applied in order.

## IR

The forms above lower to:
```js
[
	{ provider: 'kit-command', type: 'component',
	  spec: { name: 'greet', description: 'Say hello to someone', arg: 'who' } },
	{ provider: 'kit-event', type: 'event',
	  spec: { family: 'provider', name: 'providerLoaded' } },
]
```
IR fields: `provider` = exact `provider.name()`, `type` = exact `type.id()`,
`spec` = provider schema input plus the common `intent` field.

## Kit-core vs provider-owned

Kit core owns only: surface parsing, provider/type resolution, TypeBox
normalization + validation, the common `intent` field, dry-run/apply
orchestration, and event rendering.

Providers own everything else: which types exist, which fields each accepts,
whether a type can be created, generated files/edits, plans, and formatting.

**`intent` is the ONLY Kit-core spec field.** `name`, `parent`, `description`,
`path`, `method`, etc. are provider schema fields, not grammar.

## Vocabulary is dynamic

Kit loads providers *before* parsing. `kit manifest vocabulary` prints valid
example forms for the live types, generated from each `type.schema()` (field
comments come from `description`, values from `examples`/`default`/`const`).

```sh
bun run kit manifest vocabulary
bun run kit manifest vocabulary --provider kit-command
bun run kit manifest vocabulary --type event --json
```

A head like `server-db table` is invalid until a `server-db` provider with a
`table` type exists. Resolution never accepts invented heads.

## TypeBox → TCL mapping

Manifest resolution is schema-directed: after resolving the head it reads
`type.schema()` and constructs real JS values (or returns surface violations).

| TypeBox | TCL surface |
|---|---|
| `Type.String()` | `field value`, `field "value"`, or `field { multiline text }` |
| `Type.Number()` | `field 1.5` |
| `Type.Integer()` | `field 1` |
| `Type.Boolean()` | `field true` / `field false` |
| `Type.Null()` | `field null` |
| `Type.Literal('x')` | `field x` |
| `Type.Union([...])` | parsed against each branch |
| `Type.Optional(T)` | field may be absent |
| `Type.Array(T)` | repeated `field ...` or one `field { ... }` block |
| `Type.Object({...})` | `field { key value }` |
| `Type.Record(K, V)` | `field { dynamic-key { ... } }` |
| `Type.Tuple([...])` | `field { item ... }` |
| `Type.Any()`/`Type.Unknown()` | JSON-like scalar/block |

Schema annotations are enforced: object `required` keys, `additionalProperties:
false` (rejects unknown fields), `pattern`/`minLength`/`maxLength`/`minimum`,
`default` (may be applied by the normalizer), and provider metadata like
`multiple: true` (allows repeated scalars). Kit-specific metadata lives under
`kit`; `kit: { cli: false }` hides a field from the `generate` CLI but it is
still a manifest field.

### Braces are schema-directed

The same `{ }` means different things depending on the field's schema:

- `Type.String()` → braces quote multiline text (no quote-escaping needed)
- `Type.Object()` → fixed fields
- `Type.Record()` → dynamic keys
- `Type.Array()` → repeated items / nested item blocks

```kit
intent {
	Create a workspace preferences page.

	Use existing project settings layout conventions.
}

options {
	json {
		type boolean
	}
}
```

### Repeated / array values

```kit
method GET
method POST
```
lowers to `{ method: ['GET', 'POST'] }`. Equivalent braced forms:
```kit
method { GET POST }
```
```kit
method {
	GET
	POST
}
```

## The pipeline

```sh
bun run kit manifest check file.kit    # parse + resolve + full TypeBox validation, no provider run
bun run kit manifest plan  file.kit    # dry-run env: "Would create/edit ..." + pending plan preview
bun run kit manifest apply file.kit    # real env: writes files; plans are NOT executed
bun run kit manifest apply file.kit --run-plans   # writes files, then runs follow-up plans
bun run kit manifest apply - < file.kit           # stdin
bun run kit manifest <cmd> file.kit --json        # machine-readable events/operations
```

`ManifestRunner` refuses the **whole batch** if any operation is invalid — no
provider touches the workspace until every form validates. `check`/`plan` use a
dry-run env; only `apply` writes; only `apply --run-plans` runs agents.

Manifest subcommands take no ad-hoc CLI flags for provider inputs. Anything a
provider needs (including an output directory like `kit-provider`'s `project`
field) is written as a **field in the manifest form**, not a CLI flag.

## Non-goals

Manifests are not a programming language: no variables, loops, conditionals,
imports, command substitution, or shell execution, and no way to bypass
provider-owned `create()`.
