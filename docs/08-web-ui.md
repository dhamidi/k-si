# 08 — Web UI

The web UI is käsi's **fallback** interface. The main interface is email
([00](./00-vision.md)); the UI exists for the things email can't do well: initial
setup, storing secrets, editing routes/templates/skills/tools, viewing tasks, and
the one-click **task completion** page. It is deliberately small, mobile-first,
and unauthenticated at the application layer.

## The stack

A server-rendered hypermedia app, progressively enhanced — no SPA, no client
framework, minimal JavaScript.

- **[htmlc](https://github.com/dhamidi/htmlc)** (docs at
  [htmlc.sh](https://htmlc.sh)) — renders Vue Single-File-Component (`.vue`)
  syntax *server-side in Go* with no JS runtime. Components receive props from Go
  structs and evaluate a JS-compatible expression subset; directives (`v-if`,
  `v-for`, `:attr`, slots, scoped styles) work as authored. We use `RenderPage`
  for full pages and `RenderFragment` for the HTML fragments Turbo swaps in.
- **[dispatch](https://github.com/dhamidi/dispatch)** — the HTTP router: named,
  reversible routes over RFC 6570 URI templates, `net/http`-compatible. Named
  routes mean links and form actions are generated from route names, never
  hand-built strings — the URL structure can change without touching templates.
- **[Turbo](https://turbo.hotwired.dev/)** (Hotwired) — progressive enhancement
  on the client. Turbo Drive makes navigation feel instant without full reloads;
  Turbo Frames scope updates to a region; Turbo Streams let a server response
  patch several parts of the page (e.g. a task list updating after an action).
  The app remains fully functional if Turbo is absent — it just reloads.

Templates are authored as semantic, accessible HTML (correct elements, labelled
forms, proper tables) so htmlc renders clean markup that works before Turbo
enhances it.

The combination is a **hypermedia-driven** UI: the server returns HTML, the
client enhances it. This matches the dependency-light, legible-code goals — the
UI is Go structs → htmlc components → HTML, routed by dispatch, smoothed by
Turbo.

## Hosting on exe.dev, and no in-app auth

käsi runs on an **[exe.dev](https://exe.dev/) VPS** — a persistent Linux machine
with HTTPS and SSH, **private by default**, shared only through exe.dev's IAM.

Because the host keeps the deployment private, **the application ships no
authentication** — no login, no accounts, no sessions ([00](./00-vision.md)). The
security boundary is the host, not the app. This is a deliberate trade that
simplifies the entire UI, and it is recorded in several places
([06](./06-secrets.md), [04](./04-email.md)) precisely because it is load-bearing:
*if the deployment were ever made public, secrets and controls would be exposed.*
Keeping it private is an operational invariant.

Two consequences elsewhere in the design:

- **Secret management** ([06](./06-secrets.md)) is a plain UI form because the
  host gates access.
- **Capability links** — the "mark task done" URLs in emails
  ([04](./04-email.md)) — still carry an unguessable per-task token, so they work
  as one-click actions from an email client without a login step and aren't
  trivially forgeable.

## What the UI does

Scoped to the fallback role:

1. **Setup.** Configure the Fastmail account (store its API token as a secret),
   the `kasi.decode.ee` domain settings, and the **initiator allowlist** — the
   addresses allowed to start new tasks ([04](./04-email.md)). (Per-task
   collaborators are added dynamically by CC, not here — see
   [04](./04-email.md).)
2. **Secrets.** Create/update credentials, written to the secrets database
   ([06](./06-secrets.md)). The UI shows only that a secret *exists*, never its
   value.
3. **Routes, templates, skills, tools.** Bind local parts to task templates and
   edit the prompt/skills/tools those templates provision
   ([04](./04-email.md), [07](./07-skills-and-tools.md)).
4. **Tasks (read-mostly).** List and inspect tasks — status, the email thread,
   archived transcripts and artifacts ([05](./05-agents-and-tasks.md)) — as a
   fallback to reading the email thread itself.
5. **Task completion.** The tokenised page that marks a task `done` and triggers
   archive-then-cleanup ([05](./05-agents-and-tasks.md)). Often the *only* page a
   user visits in normal operation.

Skills shown here include both UI-authored ones and those an **agent wrote during
a task** ([05](./05-agents-and-tasks.md), [07](./07-skills-and-tools.md)); the UI
is where you review, edit, or retire them.

## How the UI touches the core

The UI is another edge of the runtime ([01](./01-architecture.md)), not a
side-door into the model:

- **Reads** render from the in-RAM model (and archives for heavy content) —
  fast, no query layer needed for business objects.
- **Writes** are turned into imperative **runtime messages** fed to the reducer —
  e.g. submitting the completion page emits `finish-task`; saving a route emits
  `update-route`; adding an address emits `allow-sender`. Each message is
  complete, so its handler stays pure. This keeps every state change logged and
  replayable ([01](./01-architecture.md), [03](./03-persistence.md)); the UI never
  mutates the model directly.

So a UI action and an inbound email are the same kind of thing to the core: a
message. The UI is just another message source.

## Design principles

From [00](./00-vision.md), made concrete:

- **Mobile-first.** Most interaction is a tap from an email on a phone (the
  completion link). Layouts are single-column, touch-sized, and readable on a
  small screen before they are anything else.
- **Information hierarchy first.** Every page leads with the decision or the
  primary action, then supporting detail. The completion page shows *what will
  happen* and one button. The task view leads with status and the latest
  exchange.
- **Progressive enhancement.** Works without JavaScript; Turbo makes it smooth
  when present. No capability depends on client-side scripting.
- **Small and semantic.** Few pages, semantic and accessible HTML by default. The
  UI should feel like a settings panel, not an application.
