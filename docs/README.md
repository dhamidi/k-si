# käsi — design documentation

**käsi** (Estonian/Finnish for *hand*) is an always-on, cloud-hosted agentic
assistant. You reach it by email: you send a message to a purpose-specific
address (for example `pay@kasi.decode.ee`), käsi spins up an agent to do the
work, and replies land back in your inbox as a normal email thread. A small web
UI exists for setup and secrets, but the day-to-day interface is email.

These documents describe the *design* of the system: the shape it should take,
the invariants it must hold, and the reasons behind the choices. They are meant
to stay true as the code evolves. They are not a task list or a changelog.

## Reading order

Start at the top; each doc assumes the ones above it.

| # | Document | What it covers |
|---|----------|----------------|
| 00 | [Vision & principles](./00-vision.md) | What käsi is, its goals, non-goals, and the glossary every other doc uses |
| 01 | [Runtime architecture](./01-architecture.md) | The Elm-style core: model, messages, commands, subscriptions, and event-sourced replay |
| 02 | [Object model (MIME)](./02-object-model.md) | Why MIME is the internal lingua franca, and how business objects map to it |
| 03 | [Persistence](./03-persistence.md) | SQLite layout: the message log, the inbox/outbox, archives, and the secrets DB |
| 04 | [Email & routing](./04-email.md) | Fastmail (JMAP) integration, address routing, threading |
| 05 | [Agents & tasks](./05-agents-and-tasks.md) | Task lifecycle, workspaces, harness invocation, transcripts, archival |
| 06 | [Secrets](./06-secrets.md) | The separate secrets database and `secret://` resolution |
| 07 | [Skills & tools](./07-skills-and-tools.md) | Reusable skills and CLI tools provisioned via mise |
| 08 | [Web UI](./08-web-ui.md) | The htmlc / dispatch / Turbo stack, hosted on exe.dev |
| 09 | [Code layout](./09-code-layout.md) | Package-by-domain structure and file-naming conventions |
| 10 | [End-to-end flows](./10-flows.md) | Worked walkthroughs, including the invoice-payment example |
| 11 | [The supervisor](./11-supervisor.md) | The conversational agent that drives käsi's own data model, and the control interface behind it |
| 12 | [Development process](./12-development-process.md) | The feedback loop, the twin rule, the scale envelope, and where a new developer starts |
| 13 | [Testing](./13-testing.md) | The three rings: in-memory simulation, recorded reality, and rare live probes |
| 14 | [The test language](./14-test-language.md) | The deliberately dumb, Tcl-inspired script language: send messages, assert on model fields and commands |
| 15 | [Tactical patterns](./15-tactical-patterns.md) | The canonical Go shapes: tags, payloads, constructors, handlers, effects, slices, contracts, modules |

**Joining the project?** Read [00](./00-vision.md), skim
[01](./01-architecture.md), then go straight to
[12 — Development process](./12-development-process.md); it is written as the
contributor's entry point and pulls in the rest as needed.

## The one-paragraph version

käsi is a single Go process, dependency-light, hosted on an exe.dev VPS. Its
core is an Elm-Architecture runtime — one goroutine folds a stream of
**messages** into an in-RAM **model** via pure handlers, and handlers emit
**commands** that the runtime interprets into effects, which produce more
messages. Every message is appended to a SQLite log, so state is rebuilt on
startup by replaying the log with effects suppressed. Inbound email arrives from
Fastmail over JMAP, is parsed and stored as **MIME** in an inbox table, and is
routed by the recipient's local part (`pay`, `research`, …) to a **task**. Each
task is one conversation with one agent: käsi lays the email and its attachments
into a workspace, runs a harness (Claude by default), archives the transcript,
and packages whatever the agent left behind into a MIME reply that goes out
through the outbox and back to Fastmail. Secrets live in a separate SQLite
database and are referenced by `secret://` URLs that the runtime resolves only
at the moment of effect.
