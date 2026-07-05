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
  Each view is a `web/view_<name>.vue` + `view_<name>.go` pair: the Go file
  declares the view's `<Name>View` struct and render helper — htmlc receives a
  `map[string]any`, and every value in it is one of these structs, never a raw
  model object ([15](./15-tactical-patterns.md)).
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
4. **Tasks & transcripts.** Browse tasks, drill into one, and watch its agent
   runs — including the **live transcript** of a running agent — with a button to
   **stop** an agent that's going off track (below).
5. **Task completion.** The tokenised page that marks a task `done` and triggers
   archive-then-cleanup ([05](./05-agents-and-tasks.md)). Often the *only* page a
   user visits in normal operation.
6. **Agent requests.** The tokenised form page an agent raises to collect files,
   structured fields, or secrets mid-task (below). Not a fallback — this is a
   first-class, agent-driven interaction surface.

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

Every write follows one loop, built on **form objects**
([15](./15-tactical-patterns.md)): the handler binds the request into a
`<Name>Form` (binding never fails — bad input becomes field errors), validates
it, and either **re-renders the same view** with the form carrying its values
and errors, or lets the form construct **exactly one imperative message**:

```
browser ──form──► handler: bind + validate
   │  invalid: same view re-renders; the form (values + errors) is
   │           just another struct in the props map
   ▼  valid
form.Message() ──► reducer ──► model updated
   ▼
redirect → GET → views render the new model ──► browser
```

The web edge's send blocks until the reducer has applied the message, so the
redirected `GET` always shows the new state — POST/redirect/GET with no stale
page and no client-side state.

So a UI action and an inbound email are the same kind of thing to the core: a
message. The UI is just another message source.

## Browsing tasks and transcripts

For when you're curious or impatient and want to see what an agent is actually
doing — not a fallback, a genuine window into the system.

- **Task list.** Tasks grouped by status (`awaiting-agent`, `awaiting-user`,
  `open`, `done`), newest first, each showing its route, subject, and last
  activity. Mobile-first: a scannable single column.
- **Task detail.** One task's email thread, its participants
  ([04](./04-email.md)), its agent runs, any open UI request
  ([02](./02-object-model.md)), and its artifacts.
- **Transcript view.** The agent's session transcript rendered legibly — user
  turns, assistant turns, tool calls and their results — from the harness's
  event stream ([05](./05-agents-and-tasks.md)). It reads from two places
  depending on run state:
  - a **finished** run renders from the archived transcript in SQLite
    (`archive`, `kind='transcript'` — [03](./03-persistence.md));
  - a **running** run renders from the harness's in-progress transcript in the
    workspace, so you see work as it happens.
- **Live updates.** For a running agent the transcript view **auto-updates** via
  Turbo — a refreshing Turbo Frame (or a Turbo Stream over server-sent events)
  appends new turns as the harness writes them, then settles to static once the
  run finishes. Without JavaScript it degrades to a manual refresh; nothing is
  lost.

### Stopping an agent

Each running agent run has a **Stop** button. Pressing it, like every UI write, is
just a message ([01](./01-architecture.md)):

- The `POST` emits a `stop-agent-run` message ([05](./05-agents-and-tasks.md)).
- Its effect signals the harness process; the watcher emits `finish-agent-run`
  flagged *stopped*; the transcript so far is captured; **no reply is assembled**;
  the task lands in `awaiting-user` for you to redirect or archive
  ([05](./05-agents-and-tasks.md)).

So catching an agent mid-mistake is one tap, and what it had done up to that point
is preserved for you to read. The same stop is available to the supervisor
([11](./11-supervisor.md)).

## Agent request forms

The one part of the UI the *agent* drives. When a run needs input that doesn't
belong in email — a file, several structured fields, or a secret the user
shouldn't paste into a reply — it raises a **UI request**
([02](./02-object-model.md), [05](./05-agents-and-tasks.md)). The reply email
carries a **request link**; tapping it opens a form on the web. This is faster
than an email round-trip for anything structured, and it is how secrets are
collected without ever touching email.

It is hypermedia-driven like the rest of the UI:

- **Render.** The `GET` request-link route (a dispatch named route,
  token-validated) loads the `ui_request` ([03](./03-persistence.md)) and renders
  its **form spec** with htmlc — one field per spec entry, typed as `text`,
  `longtext`, `choice`, `file` (a file input), or `secret` (a masked input). The
  agent describes *what* it needs; the page is generated from that description, so
  no bespoke page per request.
- **Submit.** A normal HTML form `POST` (progressively enhanced by Turbo). The web
  edge does all the I/O: it stores uploaded files in `archive`, writes each
  `secret` field to the secrets database and gets back a `secret://` URL
  ([06](./06-secrets.md)), and then emits a **complete** `answer-ui-request`
  message carrying only references — file archive ids and `secret://` URLs, never
  plaintext ([05](./05-agents-and-tasks.md)). The core lays the answers into `in/`
  and resumes the agent ([05](./05-agents-and-tasks.md)).
- **Once, then closed.** After a successful answer the request is `answered` and
  the link stops accepting input; re-tapping it shows the answered state. The
  token makes the link capability-bearing so it works from an email client with no
  login — the same trust model as the completion link ([04](./04-email.md)),
  which matters more here because a request may collect secrets.

Because the form is data (the spec) rendered by htmlc and posted back as a
message, the whole mechanism stays inside the hypermedia, message-sourced design —
no client app, no bespoke endpoint per request type.

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
