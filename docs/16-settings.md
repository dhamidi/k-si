# 16 — Settings & the runtime form engine

`kasi serve` accumulated flags. A few are genuine bootstrap — where the database
lives, what socket to bind — but most are *configuration*: the initiator
allowlist, the reply-from address, the runaway breakers, the public base URL. The
operator should edit those from the web UI without a redeploy, and they should
live where every other piece of durable state lives — in the model, advanced by a
logged message ([01](./01-architecture.md), [decision-001](./decision-001-ui-request-is-a-model-entry-not-a-content-table.md)).

This chapter describes how a module **contributes a typed setting**, how a **form
is generated from that setting's Go type** (and can change its own *shape* as you
fill it), and where the settings that belong to no single domain live — a new
**admin** module. The form engine it introduces is not a settings-only gadget: it
is the same runtime-form machinery the agent-request page already needs
([08](./08-web-ui.md), [decision-005](./decision-005-request-forms-are-spec-driven-with-a-nested-component-hierarchy.md)),
generalised so both surfaces render through one set of controls and parse the same
way. The design holds five lines at once — HATEOAS (no server session),
simplicity, parse-don't-validate, the shape defined once, and forms as runtime
*values* — and this chapter shows how they reinforce rather than fight each other
([decision-020](./decision-020-settings-are-typed-contributions-rendered-by-a-runtime-form-engine.md)).

**Why an engine for five settings?** Today's five settings mostly exercise the
*flat* subset — a text field, a number — which the default former handles with no
code per setting. The *dynamic* subset (a shape that grows via `Update`, list and
group kinds, the Turbo reshape) is forward-looking infrastructure, not scaffolding
for these five: it is what lets modules — and, later, the apps a user installs
([feature-apps.md](./feature-apps.md)) — contribute *rich, structured* settings
without each hand-rolling a bespoke page. The initiator allowlist demonstrates the
dynamic path so it is proven, not speculative; the rest ride the flat path until a
setting needs more. Staging it this way keeps the simplicity goal honest: the
common case stays trivial, and the machinery only appears where a setting actually
uses it.

## A setting is a typed contribution

A setting is not a config file and not a table. It is a small descriptor a domain
hands out, bundling *what the value is*, *how to read it*, and *how to write it* —
while the value itself stays in the domain's own model slice. The types live in a
new leaf package, `settings/`, which imports only `runtime/`, so any domain can
build a descriptor without an import cycle ([09](./09-code-layout.md),
[15](./15-tactical-patterns.md)):

```go
// settings/setting.go

// Setting is one typed, editable piece of configuration a module contributes.
// The value's STATE stays in the owning module's slice — contribution is not
// relocation (docs/16). The descriptor is a read + a write over that state,
// assembled in main.go (docs/01).
type Setting struct {
	Key   string // stable id, snake_case; the route param and the form's scope
	Short string // one line, shown in the settings list
	Long  string // help text, shown on the form
	Owner string // the owning module's name (email, tasks, agents, admin)

	// Read pulls the current typed value out of the model, through the owning
	// domain's pure View read helper — a read like any other (docs/15).
	Read func(v runtime.View) Value

	// Write turns an accepted value into a set-* message — the one imperative
	// write, logged and replayable (decision-001). An existing message where the
	// setting maps to one; a NEW whole-value message where it does not (a
	// list-replacing set-allowlist, not an incremental allow-sender).
	Write func(Value) runtime.Msg
}

// Value is a setting's typed value — a domain type (an Allowlist, admin.BaseURL,
// agents.MaxConcurrent), never a stringly map. It knows how to build its own
// form; that form knows how to parse back into a Value. One shape, two
// directions.
type Value interface {
	ToFormer
}
```

Each domain exposes its settings as a pure function — the contribution point:

```go
// email/settings.go

func Settings() []settings.Setting {
	return []settings.Setting{{
		Key:   "initiators",
		Short: "Addresses allowed to start new tasks",
		Long:  "The initiator allowlist (docs/04). Anyone here may open a task by email; everyone else is ignored.",
		Owner: "email",
		Read:  func(v runtime.View) settings.Value { return Allowlist(v) },
		Write: func(val settings.Value) runtime.Msg { return msg.NewSetAllowlist(...) },
	}}
}
```

`main.go` — the one assembly point ([01](./01-architecture.md)) — concatenates
them and hands the slice to the web edge. There is no global registry and no
`init()`; the settings list is built in the open, right beside the module list it
mirrors:

```go
// cmd/kasi/serve.go (the serve path)

server, err := web.NewServer(app, sec, content, work, web.Settings(
	admin.Settings(), email.Settings(), tasks.Settings(), agents.Settings(),
))
```

## ToFormer and the Form runtime value

The setting's Go type defines the form *and* the parse — the "shape defined once"
goal. A `Value` builds its form through one method:

```go
// settings/former.go

// ToFormer is implemented by a setting's Go type to define its form — the one
// shape that drives both render and parse. The DEFAULT former (below) derives
// only the OBVIOUS kinds by reflection; a type IMPLEMENTS ToForm explicitly to
// opt into a richer kind (choice, secret, file, group) or a shape that changes as
// it is filled.
type ToFormer interface {
	ToForm() Form
}
```

A `Form` is a **runtime value**, not a template: a tree of typed fields plus two
closures that make it live. `Update` changes the form's *shape* in response to an
input event; `Parse` turns the filled form back into a typed value or structured
errors — the inverse of `ToForm`:

```go
// settings/form.go

// Form is a runtime VALUE — a tree of typed fields, carried and manipulated like
// any other value, never a static template (docs/16).
type Form struct {
	Fields []Field

	// Update folds a shape-changing event into the form and returns the new
	// form: grow a list, drop a list item, reveal a field that another field's
	// value made relevant. Pure: (form, event) → form. A form with a fixed shape
	// leaves this nil.
	Update func(f Form, ev Event) Form

	// Parse reads the submitted values off the form and produces the typed Value,
	// or the per-field errors that stop the write. This is ToForm's inverse and
	// the ONLY gate — there is no separate Validate pass to diverge from it.
	Parse func(f Form) (Value, FieldErrors)
}

// Field is one control: a name, a label, a control kind chosen from the Go type,
// its current raw string (leaf controls), options (a choice), nested fields (a
// group or list item), and any parse error. It is the generalisation of
// web.FieldSpec/FieldView (decision-005), so a settings form and a Flow C request
// form render through the same control.
type Field struct {
	Name    string   // dotted path within the form: "aliases.2", "smtp.host"
	Label   string
	Kind    Kind
	Value   string   // the raw current/submitted string (leaf kinds)
	Options []string // KindChoice
	Fields  []Field  // KindGroup members, KindList items
	Error   string
}

type Kind string

const (
	KindText     Kind = "text"     // <input type=text>
	KindLongText Kind = "longtext" // <textarea>
	KindChoice   Kind = "choice"   // <select> over Options
	KindSecret   Kind = "secret"   // masked <input type=password> (decision-004)
	KindFile     Kind = "file"     // <input type=file>, stored in archive
	KindNumber   Kind = "number"   // <input type=number>, a bounded int
	KindList     Kind = "list"     // repeated Fields, add/remove controls
	KindGroup    Kind = "group"    // a nested struct's fields
)

// Event is the shape-changing input Update folds in: which list to grow or
// shrink, which dependent field to reveal.
type Event struct {
	Op    string // "add" | "remove"
	Field string // the target field's dotted path
	Index int    // for "remove": which list item
}

// FieldErrors maps a field's dotted path to its parse error — the structured,
// nested successor to web.FormErrors (a flat field→message map). Empty means the
// parse produced a value.
type FieldErrors map[string]string
```

Most settings never write a `ToForm` by hand. A **default former** derives the
*obvious* kinds by reflection, so a plain domain type gets a correct form for free
— but only the obvious ones:

```go
// settings/derive.go

// FormOf returns a value's Form: its own ToForm if it implements ToFormer,
// otherwise the default former derived from the Go type — the OBVIOUS kinds only:
//
//	string / flag.Value leaf   → KindText
//	a bounded int              → KindNumber
//	[]T                        → KindList of T's field(s), with add/remove
//
// It does NOT infer choice, secret, file, or group. There is no safe way to guess
// from a Go type that a string "is a secret" — guessing wrong leaks it — so a type
// gets those kinds only by IMPLEMENTING ToForm and saying so. Reflection covers the
// flat majority; the richer shapes are an explicit, one-method opt-in.
func FormOf(v Value) Form
```

Two examples bracket the range. `admin.BaseURL` is a leaf — the default former
gives it a single text field, and it parses through the same `flag.Value` contract
the CLI uses ([15](./15-tactical-patterns.md)):

```go
// admin/model_admin.go — the ownerless base URL, now a model value.

// BaseURL is the public origin capability links are built against (docs/04).
// It parses through flag.Value (a well-formed absolute URL) and forms through the
// default former (one text field).
type BaseURL string

func (b *BaseURL) Set(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("must be an absolute URL like https://kasi.example.com")
	}
	*b = BaseURL(raw)
	return nil
}

func (b BaseURL) String() string  { return string(b) }
func (b BaseURL) ToForm() Form     { return settings.FormOf(&b) } // default former
```

The allowlist is the dynamic case — a list whose shape grows and shrinks as you
edit it, so it implements `ToForm` explicitly to carry an `Update`. The state is
`email.Model.Allowlist`, a `[]string` today; the setting wraps it in a named
`Allowlist` type (a small refinement this change introduces, so the slice can
carry methods) and parses each row through an `addressValue` — a `flag.Value` that
validates one address, the parse-don't-validate mechanism at the element level:

```go
// email/settings.go — the allowlist owns its dynamic form. (Allowlist is a named
// []string introduced here; each row parses through the addressValue flag.Value.)

type Allowlist []string

func (a Allowlist) ToForm() settings.Form {
	f := settings.Form{Fields: rowFields(a)} // one KindText per address
	f.Update = func(f settings.Form, ev settings.Event) settings.Form {
		switch ev.Op {
		case "add":
			f.Fields = append(f.Fields, settings.Field{
				Name: fmt.Sprintf("addr.%d", len(f.Fields)), Kind: settings.KindText})
		case "remove":
			f.Fields = append(f.Fields[:ev.Index], f.Fields[ev.Index+1:]...)
		}
		return f
	}
	f.Parse = func(f settings.Form) (settings.Value, settings.FieldErrors) {
		out, errs := Allowlist{}, settings.FieldErrors{}
		for _, fld := range f.Fields {
			var addr addressValue // a flag.Value: Set validates one address
			if err := addr.Set(fld.Value); err != nil {
				errs[fld.Name] = err.Error()
			} else if addr != "" {
				out = append(out, string(addr))
			}
		}
		return out, errs
	}
	return f
}
```

`Parse` is the whole validation story for the non-sensitive fields. There is no
`Validate` method beside it to drift out of sync: a leaf parses through its
`flag.Value`, a list or group parses its children and assembles the composite,
keyed by dotted path. Parse-don't-validate is structural, not a discipline someone
has to remember.

One constraint bounds the dynamic path, and it comes from decision-004. A **secret
or file field is never part of the reshape/`Update` body**: a browser cannot be
re-seeded with a file's bytes, and a secret must never be re-rendered
([decision-004](./decision-004-secrets-are-written-at-the-web-edge-resolved-at-the-agent-edge.md)).
So a dynamic list is a list of *non-sensitive* fields (addresses, numbers), and a
setting that needs a secret uses the flat path — a fixed field set, gated at the
edge (below). This is a real limit, and it is the right one: shape-changing and
sensitive-value handling are separate concerns, and keeping them apart is what
lets the reshape round-trip re-render freely without ever echoing a secret.

## Resource in the URL, filling state in the body

The server holds **no session** ([decision-006](./decision-006-browse-ui-is-host-gated-no-app-tokens.md)):
every request carries what it needs to rebuild the form. The routes are the whole
surface (reverse-routed with `router.Path`, never hand-built —
[08](./08-web-ui.md)):

- `GET /settings` — the index: every contributed setting, its short description,
  and its current value.
- `GET /settings/{key}` — one setting's form, rendered inside a `<turbo-frame>`.
- `POST /settings/{key}/reshape` — fold one `Event` (add/remove a list item)
  through `Update` and re-render (the frame, or the whole page — below).
- `POST /settings/{key}` — the final submit: gate the sensitive fields, `Parse`,
  emit the `set-*` message, redirect.

The state split is by *role*, not by *medium*. **Resource identity** — which
setting you are editing — rides the URL: `/settings/{key}`, bookmarkable, one path
param, nothing else. **Transient filling state** — the values you have typed and
how many list rows currently exist — rides the **form body**, POSTed afresh on
every round-trip, never the query string. Two forces make this the only correct
split, not a preference. A secret-typed field's value must never land in a URL,
browser history, or an access log
([decision-004](./decision-004-secrets-are-written-at-the-web-edge-resolved-at-the-agent-edge.md));
and a reshape without JavaScript re-renders the POSTed values so nothing is lost,
which already puts filling state in the body. The server stays stateless either
way — the load-bearing HATEOAS property is *statelessness*, and both a Turbo
reshape and a full-page reshape satisfy it because the request itself carries
everything. Keeping only the resource key in the path also means there is nothing
to hand-build, so the `no-url-string-building` lint never even applies here
([08](./08-web-ui.md)).

## Reshape is a progressive enhancement, not a Turbo dependency

A shape change should feel smooth — adding an allowlist row should not blank the
page and lose what you typed — but it must also **work with no JavaScript at all**,
because in this UI nothing may depend on client-side scripting
([08](./08-web-ui.md)). Both hold, through content negotiation.

The setting form is its **own component**, renderable two ways: standalone (a
`<turbo-frame>` fragment) or embedded (inside the full page). The htmlc engine has
both render calls — `RenderPage` for the whole document and `RenderFragment` for a
bare fragment (built for "HTMX responses or turbo-frame updates", unused until
now). Note the signatures: `RenderFragment(w, name, data)` takes **no context**;
the context-carrying variant is `RenderFragmentContext(ctx, w, name, data)`.

The flow for "add an address":

1. The form renders inside `<turbo-frame id="setting-initiators">`. Each list row
   has a **remove** button; the list has an **add** button. Both are submit buttons
   whose `formaction` is the `settings.reshape` route, carrying the current values
   and the `Event` (`op=add`, or `op=remove` + `index=2`).
2. The handler binds the submitted values into the current `Form`, folds the
   `Event` through `Form.Update`, and re-binds the values onto the new shape.
3. It **content-negotiates on the `Turbo-Frame` request header**:
   - header **present** (Turbo enhanced the submit) → `RenderFragment(w,
     "view_setting", …)` returns the frame alone, and Turbo swaps it in place. The
     new row appears; every value already typed is still there; nothing else moved.
   - header **absent** (no JavaScript) → `RenderPage(ctx, w, "view_setting", …)`
     returns the whole page with the new shape and the POSTed values re-rendered —
     an ordinary full-page reload, nothing lost.

Turbo is thus a genuine *progressive enhancement*: present, it swaps a frame;
absent, the same POST reloads the page. It is introduced **minimally** — one
embedded `turbo.min.js` served on a route, a `<script>` include in `base_styles.vue`
(the per-`<head>` include each page already pulls; there is no shared layout), one
frame per setting form, and the content-negotiated reshape route. No other page
adopts frames yet. (htmx would be a lighter script, but the stack already declares
Turbo as its enhancement layer ([00](./00-vision.md), [08](./08-web-ui.md)); a
second library to save a few kilobytes is the opposite of simplicity.)

The final submit is the ordinary käsi write loop ([08](./08-web-ui.md),
[15](./15-tactical-patterns.md)): bind the body into the `Form`, run the
sensitive-field gate (below), then `Parse`; on `FieldErrors` re-render the form
(422) with the messages and the user's values; on success emit the setting's
`set-*` message (`App.Send` blocks until applied) and 303-redirect to
`GET /settings`, which renders the new value.

## Cohesion with Flow C — shared rendering and a shared gate

The agent-request page ([decision-005](./decision-005-request-forms-are-spec-driven-with-a-nested-component-hierarchy.md))
already renders a form whose fields are known only at runtime. The cohesion with
it is precise, and it is worth being exact about what is shared and what is not.
The two surfaces share **two** things, and nothing more:

**(a) Field rendering.** Both a settings form (built from a Go type by `ToForm`)
and a Flow C request form (built from the agent's JSON `FieldSpec` array by a `web`
adapter) are trees of the same `Field` kinds, rendered by the **same** control —
`request_field.vue` generalises to `field.vue`, adding the `list`/`group`/`number`
kinds while keeping the `text`/`longtext`/`choice`/`file`/`secret` controls the
request page already uses.

```go
// web/form_spec.go — the request spec becomes a settings.Form for RENDERING.

func FormFromSpec(spec []FieldSpec) settings.Form {
	f := settings.Form{Fields: make([]settings.Field, len(spec))}
	for i, s := range spec {
		f.Fields[i] = settings.Field{
			Name: s.Name, Label: s.Label, Kind: kindOf(s.Type), Options: s.Options}
	}
	return f // no Update (fixed shape), no Parse — the handler owns Flow C's submit
}
```

**(b) The decision-004 sensitive-field gate.** This is the cohesion that matters,
and it is *not* a single unified `Parse`. A secret or file field is written by the
web **edge** — `secrets.Set` returns a `secret://` URL, `content.AddArchive`
returns an `int64` ref — and the reference is substituted **before** any typed
value is built
([decision-004](./decision-004-secrets-are-written-at-the-web-edge-resolved-at-the-agent-edge.md)).
Plaintext and bytes never enter `Form.Parse`, the typed `Value`, the model, or a
re-render. So the submit path splits the fields:

```
submitted form ─► gate: secret → secrets.Set → secret://…   (edge, decision-004)
                        file   → content.AddArchive → int64
                        rest   ─────────────────────────────► Form.Parse → Value
```

`Form.Parse` therefore handles only the **non-sensitive** fields into the typed
value. A settings form with a sensitive field uses this same gate on the flat path.
And Flow C keeps its handler-side file/secret I/O **exactly as today**
([08](./08-web-ui.md)): its submit does not route through `Form.Parse` at all — it
binds the spec, runs the gate, and emits `answer-ui-request` with references only.
The request form is the flat, handler-gated path; the settings form is the
structured path with an `Update` and a `Parse` — and they meet at the field
controls and the gate, not at a single parse function.

## Which flags become settings, and which stay flags

A `kasi serve` flag becomes a model setting when it is *configuration* — something
the operator changes over the life of the deployment. It stays a pure flag when it
is *bootstrap*, *binding*, or *launch-safety*.

A setting-flag **seeds its model value only when the setting is still unset** — the
same "skip if already present" guard `seedAllowlist` uses today. This is not a
nicety; it is what makes "editable thereafter" true. Today `serve.go` re-sends
`set-reply-from`, `set-max-concurrent-runs`, and `set-loop-guard`
*unconditionally* on every boot, so a restart would **overwrite any UI edit**, and
a defaulted `-base-url` would silently re-seed and break the links a user had
corrected. Guarded seeding — seed once, then let the model own it — closes that
clobber ([decision-020](./decision-020-settings-are-typed-contributions-rendered-by-a-runtime-form-engine.md)).

| Flag | Disposition | Why |
|------|-------------|-----|
| `-base-url` | seeds if unset | The public origin; edited from the UI. Migrates edge→model (below). |
| `-allow` | seeds if unset | The initiator allowlist; the canonical edit-me config. |
| `-from` | seeds if unset | The reply-from identity; unconditional today, now guarded. |
| `-max-concurrent-runs` | seeds if unset | The OOM breaker cap ([decision-016](./decision-016-kasi-never-acts-on-its-own-mail.md)); unconditional today, now guarded. |
| `-max-task-runs` | seeds if unset | The loop breaker cap (decision-016); unconditional today, now guarded. |
| `-state` | stays a flag | Locates the log itself — you cannot read a setting to find the database that stores the settings. Bootstrap. |
| `-addr` | stays a flag | The socket bind; the control URL derives from it. Binding. |
| `-workdir` / `-spool` | stays a flag | Host paths, derivable from `-state`. Binding. |
| `-poll` / `-send` | stays a flag | They decide which *edges* `main.go` wires. Without `-send` the JMAP outbound is never constructed, so no model toggle could turn sending on — off-by-default is enforced at launch, not in the model. Safety. |

## The settings inventory

Every setting today, with the module whose state it stays in and the message that
writes it:

| Setting | Key | Owner module | Value type | Write message |
|---------|-----|--------------|------------|---------------|
| Initiator allowlist | `initiators` | `email` (`Model.Allowlist`, `[]string`) | `Allowlist` (`[]string`) | `set-allowlist` *(new)* |
| Reply-from | `reply_from` | `tasks` (`Model.ReplyFrom`) | `FromAddress` | `set-reply-from` |
| Public base URL | `base_url` | `admin` (`Model.BaseURL`) | `BaseURL` | `set-base-url` *(new)* |
| Max concurrent runs | `max_concurrent_runs` | `agents` (`Model.MaxConcurrent`) | `MaxConcurrent` (`int ≥ 0`) | `set-max-concurrent-runs` |
| Max task runs | `max_task_runs` | `tasks` (`Model.LoopGuard`) | `LoopGuard` (`int ≥ 0`) | `set-loop-guard` |

Four already live in the model; the settings surface gives them a home to be seen
and edited. Two messages are new. **`set-base-url`** comes with the migration
below. **`set-allowlist`** is new because a whole-list edit needs whole-value
*replace* semantics that the incremental `allow-sender`/`revoke-sender` cannot
express; those stay, for CC-granting and programmatic single-address writes. Both
write the same `email.Model.Allowlist` slice — the UI-replaces-vs-incremental split
memory already uses ([feature-memory.md](./feature-memory.md)), not two sources of
truth.

## The admin module, and base-url's migration

Most settings have an obvious owner — the domain whose reducers and effects read
them. The base URL does not: it is the web edge's public origin, read only to build
capability links. It has no domain, so it gets one — a new **admin** module, the
home for genuinely system-wide, ownerless configuration
([09](./09-code-layout.md)). Admin owns `Model.BaseURL`, the `set-base-url`
message, and its own `Settings()`; `main.go` wires it like any module.

The migration has a cost worth naming, because it runs against a core rule:
**effects never see the model** ([15](./15-tactical-patterns.md)). The base URL is
read inside two effects — `assemble-reply` and `mint-ui-request` — as
`email.Edges.BaseURL`, a value frozen at boot. Their *emitting* handler is `tasks`'
`run-harvest` (`replyCmds`, `message_run_harvest.go`), which already reconstructs
each reply's payload from logged model state. So the base URL moves the same way
every other fact reaches an effect: the handler reads it and threads it through the
command payload.

- `admin.Model.BaseURL` holds the value; `set-base-url` is the only writer; boot
  seeds it from the `-base-url` flag **only when unset**, so a UI edit survives a
  restart.
- `tasks`' `run-harvest` handler reads `admin.BaseURL(v)` and puts it into **both**
  `AssembleReplyPayload.BaseURL` and `MintUIRequestPayload.BaseURL` (the reply and
  the request-link branches).
- `assembleReplyEffect` and `mintUIRequestEffect` read `p.BaseURL`;
  `email.Edges.BaseURL` is deleted.

The price is one new one-directional read (`tasks` → `admin`) and two payload
fields. The payoff is that the public origin becomes logged, replayable, editable
state instead of a value you can only change by restarting the process — and
editing it in the UI changes the very next reply's link.

This does **not** endanger replay. The base URL only ever feeds a command payload
built by the `run-harvest` *handler*, never a replayed reducer; `admin.BaseURL(v)`
is read live at emit time, and where a request's capability link is already minted,
`register-ui-request` carries the built link in its logged payload (`replyCmds`
reads it back), so no effect re-derives a URL during replay. An old log with no
`set-base-url` entry converges `admin.Model.BaseURL` to empty trivially. The real
hazard the migration had to clear was the boot-seed clobber the guarded seeding
above closes, not replay
([decision-020](./decision-020-settings-are-typed-contributions-rendered-by-a-runtime-form-engine.md)).

## A setting is a kit component type

Settings are a scaffoldable shape like every other domain primitive
([15](./15-tactical-patterns.md)), so the `kasi` kit provider knows them
directly — the executable-shapes rule ([15](./15-tactical-patterns.md)): the
documented shape and the generated shape are the same shape.

- **Discovery.** `kit component list` surfaces each contributed setting as
  `kasi.setting.<key>` with its `Short` and its `<module>/settings.go` file. The
  provider finds them structurally — one ast-grep match per
  `func Settings() []settings.Setting { … }`, reading the `Key:` (and `Short:`)
  string literals out of the returned slice — so the list tracks the code with no
  registry to keep in sync.
- **Generation.** `kit generate kasi setting.<module>.<key>` scaffolds the
  descriptor and its value type. When the module has no `settings.go` yet, it
  writes the whole file — a `Settings()` returning the one descriptor plus a
  value-type skeleton (a named type with `Set`/`String` and
  `ToForm() settings.Form { return settings.FormOf(&v) }`), deterministic and
  gofmt-clean. When `settings.go` already exists, it does **not** splice into the
  returned slice literal (brittle); it writes the value-type skeleton as its own
  `setting_<key>.go` and hands the descriptor addition to the implementer as one
  `kit.Event.plan(…)` — the deterministic-file-plus-plan convention every käsi
  provider type follows. Fields: `module`, `key`, `short`, `long`, `value_type`
  (the Go value type name, defaulting to the PascalCase key), and an optional
  `message` (the `set-*` tag a write emits).

The value type the provider generates is a flat leaf — a `flag.Value` formed by
the default former — the same shape `admin.BaseURL`, `tasks.FromAddress`, and
`agents.MaxConcurrent` above already take. A dynamic setting (an `Allowlist`-style
`Update`/`Parse` form) is the implementer's follow-up on top of that leaf, not
something the scaffold guesses.
