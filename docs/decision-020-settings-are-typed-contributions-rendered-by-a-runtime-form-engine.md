# Decision 020 — settings are typed contributions rendered by a runtime form engine

## Context

`kasi serve` grew a wall of flags ([09](./09-code-layout.md),
`cmd/kasi/serve.go`). Some are genuine configuration the operator wants to change
without a redeploy — the initiator allowlist, the reply-from address, the runaway
breakers, the public base URL. Today four of those already live in the model,
seeded by a flag on boot and edited nowhere else; one, `-base-url`, is frozen into
an edge (`email.Edges.BaseURL`) and cannot be changed at all without a restart.
There is no single place to see or edit "the system's configuration."

We already have most of the machinery. The write path is settled: a form binds,
validates, and emits **one imperative message** that is logged and replayed
([08](./08-web-ui.md), [15](./15-tactical-patterns.md)); UI requests prove a
form whose fields are known only at runtime, rendered from a spec through one set
of controls
([decision-005](./decision-005-request-forms-are-spec-driven-with-a-nested-component-hierarchy.md));
`FormErrors.Parse(raw, flag.Value)` proves the "one string → typed value"
contract shared with CLI flags ([15](./15-tactical-patterns.md)); a setting is a
model entry, never a side table
([decision-001](./decision-001-ui-request-is-a-model-entry-not-a-content-table.md)).
What is missing is a way for a **module to contribute a typed setting**, a
**form derived from that type** (including forms whose *shape* changes with
input — grow a list, reveal a dependent field), and a **home** for the settings
that belong to no single domain.

The goals were fixed going in: HATEOAS (the form is rebuilt from the request, no
server session — [decision-006](./decision-006-browse-ui-is-host-gated-no-app-tokens.md));
simplicity (every mechanism earns its place); parse-don't-validate (a submission
parses straight to a typed value or structured errors, with no second validate
pass that can diverge); the shape defined once (the Go type drives render *and*
parse); and forms as runtime **values**, not templates.

## Decision

**1. A setting is a typed value a module contributes.** A new leaf package,
`settings/`, defines the engine types (`Setting`, `Value`, `ToFormer`, `Form`,
`Field`, `Event`, `FieldErrors`), importing only `runtime/` so any domain can
build descriptors without a cycle. A `settings.Setting` bundles a key, a short
and a long description, the owning module's name, a **read** (`View → Value`), and
a **write** (`Value → runtime.Msg`, a `set-*` message — an existing one where the
setting maps to one, a **new** one where the setting is a whole value no
incremental message expresses). The value's Go type is the single source of both
the form and the parse.

**2. Contribution is not relocation.** A setting's *state* stays in the module
whose reducers and effects read it — the allowlist in `email`, reply-from and the
loop guard in `tasks`, the concurrency cap in `agents`. Each domain exposes a pure
`Settings() []settings.Setting`; `main.go` — the one assembly point
([01](./01-architecture.md)) — concatenates them and hands the slice to
`web.NewServer`. There is **no global registry**: the list is assembled in the
open, like the module list beside it.

**3. A new `admin` module owns the ownerless.** Settings that belong to no domain
live in `admin/` — today exactly one, the public **base URL**, which migrates from
`email.Edges.BaseURL` (fixed at boot) to `admin.Model.BaseURL` (a logged, editable
model value). `admin` also contributes its own `Settings()`. The aggregating
`/settings` UI is *rendered* in `web/`, where all rendering lives
([09](./09-code-layout.md), [decision-002](./decision-002-ui-requests-live-in-the-tasks-domain.md)).

**4. `ToFormer` and the `Form` runtime value.** A `Value` builds its form via
`ToForm() Form`. `Form` is a runtime value — a tree of typed `Field`s carrying two
closures: `Update(Form, Event) Form`, which changes the form's **shape** in
response to an input event (add/remove a list item, reveal a dependent field) and
returns a new form; and `Parse(Form) (Value, FieldErrors)`, the inverse of
`ToForm` — one shape, two directions. A **default former** derives only the
*obvious* kinds by reflection — a `string`/`flag.Value` leaf → text, a bounded int
→ number, a `[]T` → a list of the element's fields with add/remove. It deliberately
does **not** guess the richer kinds: a type gets `choice`, `secret`, `file`, or a
nested `group` only by **implementing `ToForm` explicitly**. Reflection guessing
"which string is a secret" would be a security foot-gun; opting in is one method.

**5. One rendering engine, a shared decision-004 gate — cohesion with Flow C.**
`settings.Form` is the common **rendering** currency: both the settings form (built
from a Go type by `ToForm`) and the Flow C request form (built from the agent JSON
spec by a `web` adapter, [decision-005](./decision-005-request-forms-are-spec-driven-with-a-nested-component-hierarchy.md))
are trees of the same `Field` kinds, rendered by the **same** control
(`request_field.vue` generalises to `field.vue`). Cohesion is two things, and no
more: (a) shared field rendering, and (b) a shared **sensitive-field gate**. It is
*not* a single unified `Parse`. Secret and file fields never reach `Form.Parse`:
the web edge writes each secret to `secrets.Set` (→ a `secret://` URL) and each
upload to `content.AddArchive` (→ an `int64` ref) and substitutes the reference
**before** the typed value is built
([decision-004](./decision-004-secrets-are-written-at-the-web-edge-resolved-at-the-agent-edge.md)) —
plaintext and bytes never enter `Form.Parse`, the typed `Value`, the model, or a
re-render. `Form.Parse` handles only the non-sensitive fields into the typed value.
Flow C keeps its handler-side file/secret I/O exactly as today; a settings form
with a sensitive field uses the same gate and the same flat path.

**6. Resource in the URL, filling state in the body — sessionless.** The server
holds **no session** ([decision-006](./decision-006-browse-ui-is-host-gated-no-app-tokens.md)):
every request carries everything needed to rebuild the form. The split is by role.
*Resource identity* rides the URL — `GET /settings/{key}` names which setting,
bookmarkable, reverse-routed with `router.Path` ([08](./08-web-ui.md)). *Transient
filling state* — the in-progress values and the current row count — rides the
**form body** on every round-trip, never the query string. Two reasons force this:
a secret-typed field's value must never touch a URL
([decision-004](./decision-004-secrets-are-written-at-the-web-edge-resolved-at-the-agent-edge.md)),
and a no-JS reshape (below) re-renders the POSTed values to avoid losing them,
which already puts filling state in the body. The server stays stateless either
way — the load-bearing HATEOAS property — and there is no URL-string-building
tension because nothing but the resource key is in the path.

**7. Reshape is a progressive enhancement, not a Turbo dependency.** A
shape-changing control POSTs the current field values plus an `Event` to
`POST /settings/{key}/reshape`; the handler folds the event through `Update` and
re-renders. It **content-negotiates on the `Turbo-Frame` request header**: present
→ render the frame alone with the htmlc `RenderFragment` path (present but unused
until now, [08](./08-web-ui.md)) and Turbo swaps it inline; absent → render the
**full page** with `RenderPage`, POSTed values re-rendered so nothing is lost, a
plain full-page reload. The setting form is its own component, renderable
standalone (a frame fragment) or embedded (in the page), so both paths share one
template. This honours docs/08's "no capability depends on client-side scripting":
without JavaScript the reshape still works, it just reloads.

**8. Parse-don't-validate, for real.** `Form.Parse` is the single gate for the
non-sensitive fields: it returns a typed `Value` or `FieldErrors`, and there is no
separate `Validate` step to diverge from it. `FormErrors.Parse(raw, flag.Value)`
generalises to the nested case — a leaf still parses through `flag.Value`, a
group/list parses its children and assembles the composite value, keyed by dotted
path (`aliases.2`). Final submit `POST /settings/{key}` runs the sensitive-field
gate, then `Parse`; on success it emits the setting's `set-*` message (`App.Send`
blocks until applied) and 303-redirects to `GET /settings`; on failure it
re-renders the form with `FieldErrors`.

**9. Flags seed conditionally, or stay flags.** A flag that is configuration seeds
its model setting **only when the setting is still unset in the model** — the exact
"skip if already present" guard `seedAllowlist` already uses (`cmd/kasi/serve.go`).
This is load-bearing: today `serve.go` re-sends `set-reply-from`,
`set-max-concurrent-runs`, and `set-loop-guard` *unconditionally* every boot, so a
restart would clobber any UI edit, and a defaulted `-base-url` would silently
re-seed and break links. Guarded seeding is what makes "editable thereafter" true:
`-base-url`, `-allow`, `-from`, `-max-concurrent-runs`, `-max-task-runs` seed once
and are UI-owned after. A flag that is **bootstrap, binding, or launch-safety**
stays a pure flag: `-state` (locates the log — you cannot read a setting to find
the database that stores it), `-addr` (socket bind; the control URL derives from
it), `-workdir`/`-spool` (host paths, derivable from `-state`), and `-poll`/`-send`
(they change which *edges* `main.go` wires — without `-send` the JMAP outbound is
never constructed, so no model toggle could turn it on).

## Consequences

- **New package `settings/`** (leaf, imports `runtime/` only): `setting.go`
  (`Setting`, `Value`), `former.go` (`ToFormer`), `form.go` (`Form`, `Field`,
  `Kind`, `Event`, `FieldErrors`), `derive.go` (the reflection default former —
  text/number/list only), `parse.go` (the nested `flag.Value` parse). It is
  domain-agnostic, like `testlang/` and the `msg/` leaves.
- **New module `admin/`**: `model_admin.go` (`BaseURL` + its `flag.Value` value
  type, plus an explicit `ToForm` — a leaf, so the default former suffices),
  `message_set_base_url.go` (`set-base-url` + handler), `admin/msg` (the contract
  others seed), `settings.go` (`admin.Settings()`). Wired in `main.go` like every
  other module.
- **A whole-value setting gets a whole-value write.** The allowlist edit replaces
  the list, which no incremental message expresses, so a **new** `set-allowlist`
  (replace semantics) is added. The incremental `allow-sender`/`revoke-sender` stay
  for CC-granting and programmatic writes; both write the same
  `email.Model.Allowlist` slice — the UI-replaces-vs-agent-appends split memory
  already uses (feature-memory.md). `set-reply-from`, `set-loop-guard`,
  `set-max-concurrent-runs`, and the new `set-base-url` are already whole-value.
- **base-url migration has a real cost, stated plainly.** Effects never see the
  model ([15](./15-tactical-patterns.md)). `assemble-reply` and `mint-ui-request`
  read the base URL inside their effects (`email.Edges.BaseURL`); their *emitting*
  handler is `tasks`' `run-harvest` (`replyCmds` and the `HarvestRequest` branch,
  `message_run_harvest.go`). So the handler now reads `admin.BaseURL(v)` and threads
  it through both `AssembleReplyPayload.BaseURL` and `MintUIRequestPayload.BaseURL`;
  the two effects read `p.BaseURL`, and `email.Edges.BaseURL` is deleted. This adds
  one one-directional read (`tasks` → `admin`) and two payload fields — the price of
  making a boot-frozen edge into logged, editable state.
- **Replay stays safe** — the migration touches a command payload built by a
  *handler*, never a replayed reducer. `admin.BaseURL(v)` is read live by
  `run-harvest` when it emits the command; and where a request's link is already
  minted, `register-ui-request` carries the built `Link` in its logged payload
  (`replyCmds` reads it back), so no effect re-derives a URL during replay. An old
  log with no `set-base-url` entry converges `admin.Model.BaseURL` to empty
  trivially. The genuine risk here was never replay — it was the boot-seed clobber
  that ruling 9 above closes.
- **`web` gains the settings surface**: `view_settings.vue`/`view_settings.go` (the
  index), `view_setting.vue`/`view_setting.go` (one setting form, renderable
  standalone as a frame or embedded in the page), `field.vue` (the generalised
  control, replacing `request_field.vue`), `form_setting.go` (the sensitive-field
  gate → `Form.Parse` → one message), and routes `settings.index`, `settings.show`,
  `settings.reshape`, `settings.save`. The Flow C request page re-expresses on
  `field.vue` without regressing decision-004/005 — its handler-side file/secret I/O
  is unchanged.
- **Turbo is introduced minimally**: one embedded `turbo.min.js` served on a route,
  a `<script>` include in `base_styles.vue` (there is no shared layout;
  `base_styles.vue` is the per-`<head>` include each page already pulls), one
  `<turbo-frame>` per setting form, and the content-negotiated reshape route.
  Nothing else adopts frames yet; the pages degrade to full reloads without
  JavaScript ([08](./08-web-ui.md)).
- **Serve keeps the seed flags** (they still parse and validate at boot — the
  `-send` guard against a `.test` base URL stays), but the runtime value is the
  model's and seeding is guarded. `-base-url` seeds `set-base-url` only when unset,
  exactly as the other seeds become guarded.

## Coverage

- `t/web/settings.test` — `visit /settings` lists every contributed setting with
  its short description and current value; `visit /settings/base_url` renders the
  form; a valid `post /settings/base_url` emits `set-base-url` and the reloaded
  index shows the new value (the `visit` conformance every new page owes,
  [decision-008](./decision-008-web-render-is-tested-with-an-in-process-visit-vocab.md)).
- `t/web/settings-reseed-guard.test` — a `set-*` edit followed by a simulated
  reboot's seed leaves the edited value intact (the seed is skipped because the
  setting is set) — the anti-clobber contract.
- `t/web/settings-reshape.test` — a reshape POST that adds an allowlist row returns
  the frame fragment with one more field and the previously-typed values intact; a
  remove shrinks it; and the same POST without the `Turbo-Frame` header returns the
  full page with the values preserved — reshape degrades without JS.
- `t/web/settings-parse.test` — an invalid value (a non-URL base URL, a negative
  cap) re-renders the form with the `FieldErrors` message and emits nothing;
  parse-don't-validate at the value level.
- `t/mail/base-url-from-model.test` — a reply's capability link is built from
  `admin.BaseURL`, and editing it through `set-base-url` changes the next reply's
  link (the migration is real, not cosmetic).

## Related

- [decision-001](./decision-001-ui-request-is-a-model-entry-not-a-content-table.md)
  — a setting is a model entry advanced by a message, no side table.
- [decision-004](./decision-004-secrets-are-written-at-the-web-edge-resolved-at-the-agent-edge.md)
  — the sensitive-field gate the settings form reuses; why secret/file values never
  reach `Form.Parse` and never touch a URL.
- [decision-005](./decision-005-request-forms-are-spec-driven-with-a-nested-component-hierarchy.md)
  — the runtime-fielded form whose rendering the engine shares; Flow C stays the
  flat, handler-gated path.
- [decision-006](./decision-006-browse-ui-is-host-gated-no-app-tokens.md) — no
  session, host-gated; the settings pages carry no token.
- [decision-008](./decision-008-web-render-is-tested-with-an-in-process-visit-vocab.md)
  — every new page ships a `visit` assertion.
- [16](./16-settings.md) — the chapter that walks the engine end to end.
- `cmd/kasi/serve.go` (the seed flags, now guarded), `email/module.go` /
  `email/command_assemble_reply.go` / `email/command_mint_ui_request.go` (the
  base-url edge, removed), `tasks/message_run_harvest.go` (`replyCmds`, now threads
  base-url), `web/server.go` (the new routes and the content-negotiated fragment
  path).
