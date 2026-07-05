# 00 — Vision & principles

## What käsi is

käsi is a personal, always-on assistant that lives in the cloud and works
through email. You delegate a piece of work by sending an email to a
purpose-built address; käsi runs an agent to do it and replies in the same
thread. When the agent needs more from you, it asks — by replying — and you
answer by replying. When the work is done, you close the thread with a single
click.

It is *agentic* (it runs real agents that use real tools, not a chatbot),
*always-on* (a long-lived process, not a request/response function), and
*yours* (single-tenant, hosted on your own VPS, wired to your own mailbox).

## Goals

- **Email is the primary interface.** Every capability must be reachable by
  sending and receiving email. The web UI is a fallback, never a requirement for
  normal use.
- **One task, one conversation, one agent session.** A task is a durable unit of
  work that maps one-to-one onto an email thread and onto a single agent
  session. This equivalence is load-bearing and appears throughout the design.
- **Durable by construction.** The system can be killed at any moment and
  restarted without losing state or double-performing side effects.
- **Dependency-light Go.** Prefer the standard library and small, legible
  packages. Every dependency is a liability to be justified. The known external
  building blocks are listed under *Dependencies* below.
- **Flexible internal representation.** Use MIME as the internal object model so
  that anything email can carry — text, attachments, headers, nested
  messages — is a first-class citizen without a bespoke schema.
- **Legible, domain-shaped code.** Structure the code by business domain and
  name files after the single thing they contain, so the layout reads like a
  description of the system (see [09](./09-code-layout.md)).

## Non-goals

- **Multi-tenancy and in-app auth.** käsi is single-user. Access control is the
  host's job (exe.dev keeps the VPS private; see [08](./08-web-ui.md)). The
  application ships no login, no user accounts, no session cookies.
- **A rich web application.** The UI does setup, secrets, and a fallback view of
  tasks. It is deliberately small. Anything that can be an email should be one.
- **A general workflow engine.** käsi orchestrates agent runs against email, not
  arbitrary DAGs. Routing is by email address; sequencing is by conversation.
- **Horizontal scale.** One process, one machine, business objects in RAM. If it
  ever needs to scale out, that is a redesign, not a config change.

## The scale envelope

käsi is small on purpose, and the numbers are part of the design:

- **At most ~20 people** interact with an instance — the owner plus
  participants CC'd into threads ([04](./04-email.md)) — with only a handful
  actively messaging at any moment.
- **~100 concurrent agent runs** is the load the system must handle
  comfortably: many slow, long-running worker processes against a trickle of
  human traffic.

Every sizing decision — one process, business objects in RAM, full-log replay,
SQLite — is justified against this envelope, and the test fleet exercises the
system beyond it ([13](./13-testing.md)). If the envelope ever grows by an
order of magnitude, revisit the design ([12](./12-development-process.md));
do not pre-build for scale that may never come.

## Principles

1. **Effects are described, then interpreted.** Handlers never perform I/O. They
   return *commands* — data — and the runtime performs the effect. This is what
   makes replay safe and testing trivial.
   - **Messages are imperative and complete.** Every runtime message is phrased
     in the imperative mood — it *tells the model what to do* (`route-email`,
     `finish-agent-run`, `mark-email-sent`) — always, no exceptions. And it
     carries *everything its handler needs* to compute the next state. A handler
     never reaches out for more data: no file read, no query, no clock. All I/O
     already happened at the edge that produced the message. This is what keeps
     handlers pure and replay deterministic (see [01](./01-architecture.md)).
2. **The log is the source of truth for state; SQLite tables are the source of
   truth for content.** The message log reconstructs the model. The inbox,
   outbox, archives, and secrets are durable stores the model points into. Keep
   the two roles distinct (see [03](./03-persistence.md)).
3. **Open sets, not closed unions.** The set of message and command types is
   open. An unknown message or command is dropped, not an error. This lets
   handlers be added and removed without a central registry and lets old logs
   replay against new builds.
4. **Determinism in, effects out.** Anything non-deterministic (time, random
   IDs, network results) enters the model *as a message*, so replay never has to
   reproduce it.
5. **Secrets are references until the last moment.** The model, the log, and
   workspaces hold `secret://` URLs, never plaintext. Resolution happens inside
   the command interpreter, at the edge (see [06](./06-secrets.md)).
6. **Design for the small screen and the skimming reader.** Email and UI alike
   lead with the decision or the ask, then the detail. Mobile-first layout,
   clear information hierarchy.

## Glossary

These terms are used precisely throughout the docs. The word "message" is
overloaded in the problem domain (email messages vs. runtime events), so we
always qualify it.

| Term | Meaning |
|------|---------|
| **Runtime message** (`Msg`) | An imperative, self-complete instruction fed into the Elm-style core. Pure input to handlers. Every one is appended to the message log. Never confuse with a MIME message. |
| **Command** (`Cmd`) | A *description* of an effect, returned by a handler. Interpreted by the runtime, which performs the effect and feeds results back as runtime messages. |
| **Subscription** (`Sub`) | A declared, long-lived source of runtime messages (a poller, a timer, an event stream) that the runtime keeps alive while the model asks for it. |
| **Model** | The entire in-RAM application state. A pure fold of all runtime messages. |
| **Message log** | The append-only SQLite table of every runtime message, in order. Replayed on startup to rebuild the model. |
| **MIME message** | An email-shaped object: headers plus a (possibly multipart) body. käsi's internal object model. Stored in the inbox/outbox and archives. |
| **Inbox / Outbox** | SQLite tables of inbound / outbound MIME messages. The durable boundary between käsi and Fastmail. |
| **Task** | One unit of work = one email thread = one agent session. The central business object. |
| **Workspace** | The on-disk directory for a task, `$WORKDIR/task-$ID`, with `in/` and `out/` subdirectories. |
| **Agent run** | A single invocation of a harness (Claude by default) inside a workspace. |
| **Main agent / orchestrator** | käsi itself, in its role of managing the inbox/outbox of every agent run. Its conversational face is the *supervisor*. |
| **Supervisor** | The conversational agent you email to drive käsi's own data model — list/inspect/stop/resume/archive tasks and UI requests ([11](./11-supervisor.md)). |
| **Control interface** | A loopback endpoint the runtime exposes so out-of-process edges (the supervisor's CLI) can read the model and inject messages ([11](./11-supervisor.md)). |
| **Harness** | The official runner for an agent (e.g. the Claude CLI/SDK). käsi shells out to it; it does not reimplement one. |
| **Handler (route)** | The mapping from an email local part (`pay`, `research`, …) to a task template. |
| **Task template** | The prompt, skills, and tools that define a category of work. Selected by the route. |
| **UI request** | An agent-raised web form (files / fields / secrets) delivered as a tokenised link in an email reply and answered on the web. |
| **Skill** | A reusable instruction/prompt bundle made available to agent runs. |
| **Tool** | A CLI program provisioned into a workspace via mise. |
| **Initiator allowlist** | The set of email addresses permitted to *start* a new task. A global gate on new conversations. |
| **Participant / collaborator** | An address authorised to interact with *one existing task*, granted by being CC'd on an authorised message. Task-scoped; does not grant initiation rights. |
| **Secret** | A credential stored in the separate secrets database, addressed by a `secret://` URL. |

## Dependencies

Deliberately small. Anything not here should be questioned.

- **Go standard library**, including `net/mail` / `mime` for MIME and
  `net/http` for the web and JMAP clients.
- **SQLite** via `mattn/go-sqlite3` — the only datastore.
- **mise** — installs and pins the CLI tools that agent runs need.
- **The agent harness** — the Claude CLI/SDK by default; other official
  harnesses are pluggable.
- **Web stack**: [htmlc](https://github.com/dhamidi/htmlc) (server-side Vue-SFC
  rendering in Go; docs at [htmlc.sh](https://htmlc.sh)),
  [dispatch](https://github.com/dhamidi/dispatch) (named, reversible routing),
  and [Turbo](https://turbo.hotwired.dev/) (progressive enhancement). See
  [08](./08-web-ui.md).
- **Fastmail** over JMAP for all email I/O. See [04](./04-email.md).
- **exe.dev** as the host. See [08](./08-web-ui.md).
