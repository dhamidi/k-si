# Brief — settings/admin module + runtime form engine

Implementation synthesis for [docs/16](docs/16-settings.md) /
[decision-020](docs/decision-020-settings-are-typed-contributions-rendered-by-a-runtime-form-engine.md).
Docs are the "why"; this is the ordered build. No behavior beyond what the docs
specify.

## 0. Settings inventory & flag split

| Setting | Key | Owner (state) | Value type | Write msg | Flag today |
|---|---|---|---|---|---|
| Initiator allowlist | `initiators` | `email.Model.Allowlist` (`[]string`) | `Allowlist` (`[]string`) | `set-allowlist` *(new, replace)* | `-allow` (seeds if unset) |
| Reply-from | `reply_from` | `tasks.Model.ReplyFrom` | `FromAddress` | `set-reply-from` | `-from` (seeds if unset) |
| Public base URL | `base_url` | `admin.Model.BaseURL` *(new)* | `BaseURL` | `set-base-url` *(new)* | `-base-url` (seeds if unset; edge→model) |
| Max concurrent runs | `max_concurrent_runs` | `agents.Model.MaxConcurrent` | `MaxConcurrent (int≥0)` | `set-max-concurrent-runs` | `-max-concurrent-runs` (seeds if unset) |
| Max task runs | `max_task_runs` | `tasks.Model.LoopGuard` | `LoopGuard (int≥0)` | `set-loop-guard` | `-max-task-runs` (seeds if unset) |

`set-allowlist` is **new**: a whole-list edit needs replace semantics that
incremental `allow-sender`/`revoke-sender` can't express. Keep those two for
CC-granting and programmatic single-address writes; both write the same
`email.Model.Allowlist` slice (UI-replace vs incremental, like memory).

Stay pure flags: `-state` (bootstrap: locates the log), `-addr` (binding; control
URL derives), `-workdir`/`-spool` (derivable from `-state`), `-poll`/`-send`
(launch-safety: decide which edges `main.go` wires — a model toggle can't
construct the JMAP outbound).

## 1. `settings/` engine package (leaf, imports `runtime/` only)

- [ ] `settings/setting.go` — `Setting{Key, Short, Long, Owner string; Read func(runtime.View) Value; Write func(Value) runtime.Msg}`; `Value interface { ToFormer }`.
- [ ] `settings/former.go` — `ToFormer interface { ToForm() Form }`.
- [ ] `settings/form.go` — `Form{Fields []Field; Update func(Form, Event) Form; Parse func(Form) (Value, FieldErrors)}`; `Field{Name, Label string; Kind Kind; Value string; Options []string; Fields []Field; Error string}`; `Kind` consts `text|longtext|choice|secret|file|number|list|group`; `Event{Op string; Field string; Index int}`; `FieldErrors map[string]string` (dotted-path keys).
- [ ] `settings/derive.go` — `FormOf(v Value) Form`: default former derives **only the obvious kinds** by reflection — `string`/`flag.Value`→text, bounded int→number, `[]T`→list. It does NOT infer choice/secret/file/group (no safe way to guess "this string is a secret"); a type opts into those by IMPLEMENTING `ToForm` explicitly.
- [ ] `settings/parse.go` — nested generalisation of `web.FormErrors.Parse(raw, flag.Value)`: leaf parses via `flag.Value`; group/list parses children, assembles composite, keys errors by dotted path. Parses **non-sensitive fields only** (secret/file are gated at the edge — §5).

## 2. `admin/` module (new domain; ownerless settings)

- [ ] `admin/model_admin.go` — `type BaseURL string` impl `flag.Value` (`Set` rejects non-absolute URL) + explicit `ToForm() Form { return settings.FormOf(&b) }` (a leaf, so the default former suffices); `Model{BaseURL BaseURL}`; read helper `func BaseURL(v runtime.View) BaseURL`.
- [ ] `admin/msg/set_base_url.go` — `const SetBaseURL = "set-base-url"`, payload, `NewSetBaseURL`.
- [ ] `admin/message_set_base_url.go` — handler: writes `Model.BaseURL`.
- [ ] `admin/module.go` — `Module(Edges) *runtime.Module` registering the above; `SimEdges()`.
- [ ] `admin/settings.go` — `Settings() []settings.Setting` (the `base_url` descriptor).

## 3. base-url edge→model migration (replay-safe — payload-fed, not reducer-fed)

- [ ] Add `BaseURL string` to `email/msg/assemble_reply.go` `AssembleReplyPayload` and `email/msg/mint_ui_request.go` `MintUIRequestPayload`.
- [ ] `tasks/message_run_harvest.go`: `replyCmds` (the `HarvestReply` path) AND the `HarvestRequest` branch read `admin.BaseURL(v)` and thread it into both payloads. (New one-directional read `tasks`→`admin`; pass `runtime.View` into `replyCmds` if not already available.)
- [ ] `email/command_assemble_reply.go` / `email/command_mint_ui_request.go`: read `p.BaseURL` instead of `e.BaseURL`.
- [ ] Delete `email.Edges.BaseURL` (`email/module.go`, its `SimEdges`) and the `main.go` wiring `BaseURL: *baseURL`.
- [ ] No replay risk: `admin.BaseURL(v)` is read live by the harvest handler at emit time; an already-minted request link rides `register-ui-request`'s logged payload; old logs with no `set-base-url` converge to empty base-url trivially.

## 4. Per-domain `Settings()` contributions

- [ ] `email/settings.go` — `initiators` (Read `Allowlist(v)`, Write `set-allowlist`). Introduce a named `type Allowlist []string` (so the slice can carry methods — a small refinement over the `[]string` model field) with an explicit `ToForm` carrying `Update` (add/remove rows) + `Parse`; each row parses through an `addressValue` `flag.Value` (element-level parse-don't-validate). Constraint: dynamic list rows are **non-sensitive** (addresses) — decision-004 keeps secret/file out of the reshape/Update body.
- [ ] `email/message_set_allowlist.go` + `email/msg/set_allowlist.go` — `set-allowlist` (replace `Model.Allowlist`).
- [ ] `tasks/settings.go` — `reply_from` + `max_task_runs`.
- [ ] `agents/settings.go` — `max_concurrent_runs`.
- [ ] Value types (`FromAddress`, `MaxConcurrent`, `LoopGuard`) impl `flag.Value` (most already do); flat leaves get their form via `settings.FormOf` (text/number) — no explicit `ToForm` needed.

## 5. `web/` settings surface (rendering lives here; cohesion = shared render + shared decision-004 gate)

- [ ] `web/field.vue` — generalise `request_field.vue`: add `number`/`list`/`group` kinds; keep `text|longtext|choice|file|secret`. Point `request_*.vue` at it; delete `request_field.vue`.
- [ ] `web/form_spec.go` — add `FormFromSpec([]FieldSpec) settings.Form` for **rendering only** (no `Update`, no `Parse`). Flow C's submit is UNCHANGED: it keeps its handler-side file→`content.AddArchive`/secret→`secrets.Set` I/O and emits `answer-ui-request` with references only (decision-004/005). Do NOT route Flow C through `Form.Parse`.
- [ ] Shared **sensitive-field gate** (used by settings submit AND kept by Flow C): before any typed value is built, secret fields → `secrets.Set` (→ `secret://`), file fields → `content.AddArchive` (→ `int64`); substitute references; plaintext/bytes never reach `Form.Parse`, the model, or a re-render.
- [ ] `web/view_settings.{go,vue}` — `SettingsView` (index: list of {Key, Short, current value, edit path}), `RenderSettings`.
- [ ] `web/view_setting.{go,vue}` — the setting form as its **own component**, renderable standalone (a `<turbo-frame id="setting-{key}">` fragment) OR embedded in the full page, so one template serves both the fragment and the reload path.
- [ ] `web/form_setting.go` — bind body → sensitive-field gate → `Form.Parse` (non-sensitive); invalid re-renders (422) with `FieldErrors`; valid → `setting.Write(value)` → one message.
- [ ] `web/server.go` — extend `NewServer` to take `[]settings.Setting`; add routes (all via `router.Path`; host-gated, no token — decision-006):
  - `GET settings.index /settings` → index (`visit` conformance).
  - `GET settings.show /settings/{key}` → the form page (frame embedded).
  - `POST settings.reshape /settings/{key}/reshape` → bind values + `Event`, fold `Update`, then **content-negotiate on the `Turbo-Frame` request header**: present → `RenderFragment(w, "view_setting", …)` (the frame); absent → `RenderPage(ctx, w, "view_setting", …)` (full page, POSTed values preserved). NOTE signature: `RenderFragment` takes **no ctx** (ctx variant is `RenderFragmentContext`).
  - `POST settings.save /settings/{key}` → gate → `Parse` → message → 303 to `settings.index`.
- [ ] State split: **resource identity** (which setting) in the URL `/settings/{key}`; **filling state** (values, row count) in the form body every round-trip — never the query string (secrets must never touch a URL; no-JS reshape re-renders POSTed body). Server stays sessionless either way.

## 6. Turbo (minimal, net-new; progressive enhancement)

- [ ] Embed `turbo.min.js` (pin a version), serve on a `GET assets.turbo` route.
- [ ] `<script src>` for it in `base_styles.vue` (the per-`<head>` include each page already pulls; there is no shared layout).
- [ ] Reshape controls = submit buttons with `formaction` → `settings.reshape`, inside the frame. With JS, Turbo swaps the frame; without JS, the same POST reloads the whole page (content negotiation in §5). No capability depends on scripting.

## 7. `cmd/kasi/serve.go` — GUARDED seeding (the anti-clobber fix)

- [ ] Wire `admin.Module(...)` into `runtime.New(...)`.
- [ ] **Change the unconditional `set-*` re-seeds to guarded / only-if-unset.** Today `serve.go` sends `set-reply-from`, `set-max-concurrent-runs`, `set-loop-guard` on EVERY boot — a restart clobbers UI edits. Mirror `seedAllowlist`'s "skip if already present": seed each only when the setting is unset in `app.View()`.
- [ ] Seed `-base-url` via `app.Send(adminmsg.NewSetBaseURL(...))` **only when unset** (a default `-base-url` would otherwise re-seed every boot and break edited links). Keep the `-send` boot guard against a `.test` base URL against the flag seed.
- [ ] `web.NewServer(app, sec, content, work, web.Settings(admin.Settings(), email.Settings(), tasks.Settings(), agents.Settings()))`.
- [ ] Leave `-state/-addr/-workdir/-spool/-poll/-send` untouched.

## 8. Tests (scenario scripts; `visit` per decision-008)

- [ ] `t/web/settings.test` — `visit /settings` lists settings + current values; `visit /settings/base_url` renders form; valid `post /settings/base_url` emits `set-base-url`, reloaded index shows new value.
- [ ] `t/web/settings-reseed-guard.test` — a `set-*` edit followed by a simulated reboot's seed leaves the edited value intact (seed skipped because set) — the anti-clobber contract.
- [ ] `t/web/settings-reshape.test` — reshape POST WITH `Turbo-Frame` header adds an `initiators` row → fragment has one more field, previously-typed values intact; remove shrinks it; the SAME POST WITHOUT the header returns the full page with values preserved (degrade-without-JS).
- [ ] `t/web/settings-parse.test` — invalid value (non-URL base URL / negative cap) re-renders form with `FieldErrors`, emits nothing.
- [ ] `t/mail/base-url-from-model.test` — reply link built from `admin.BaseURL`; `set-base-url` changes the next reply's link.
- [ ] Keep the existing Flow C request tests green on the shared `field.vue` (rendering) and unchanged handler-side gate.
- [ ] `mise run check` (full gate, `--log sqlite` replay converges the new `admin` model field).

## 9. kit provider (docs/15 executable-shapes rule)

- [ ] Add generators/templates for `settings.go` contributions and the `admin`-style ownerless-setting module so `kit component list`/`kit generate` know the new shapes; fix docs + provider in the same change if they disagree.
