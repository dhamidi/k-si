# Building käsi — the order of construction

This is the itinerary, for a team of humans and agents building käsi together.
The design documents in [docs/](./docs/README.md) are the law and stay
evergreen; this file sequences the work and is the one planning document that
changes as stages complete. When this file and the docs disagree, the docs
win.

The first destination, deliberately unrushed: **a full multi-turn email
conversation that works — including file attachments.** Everything else,
including the web UI, grows on top of that.

## Working rules

Binding for every implementer, human or agent:

1. **Read before writing.** [00](./docs/00-vision.md) and
   [01](./docs/01-architecture.md) for the system,
   [12](./docs/12-development-process.md)–[15](./docs/15-tactical-patterns.md)
   for how we work. A module's owner also reads that module's design doc.
2. **Contracts first.** Cross-module work starts by committing the receiving
   module's `msg/` package ([15](./docs/15-tactical-patterns.md)). After
   that, both sides are compiler-checked and can proceed in parallel.
3. **Flow scripts are frozen.** The scenario scripts under `t/` that encode
   the flows of [10](./docs/10-flows.md) are written by the architect role
   *before* the modules they exercise, and are never edited by whoever (or
   whatever) makes them pass. Changing one is a design change and is reviewed
   as such ([12](./docs/12-development-process.md)).
4. **Scaffold through kit, never by hand.** `kit manifest apply` /
   `kit generate` (the `kasi` provider, `providers/kasi/`) create every
   module, message, command, model object, and subscription. The provider is
   idempotent (re-apply never clobbers implemented code), refuses duplicate
   tags, registers into `module.go`, and wires modules into `main.go` — the
   divergences it prevents are exactly the ones hand-editing invites.
5. **`kit component list` is the shared map.** Consult it before inventing a
   tag, and `kit component show` before touching someone else's module.
6. **The gates are mechanical, not advisory.** Before merging:
   `kasi test t/` green, `ast-grep scan` clean (the rule pack in `rules/`
   enforces [15](./docs/15-tactical-patterns.md)), and no `*_test.go`
   anywhere — the scripts are the tests.
7. **One module, one owner at a time.** Parallelism happens *across*
   modules, against contracts — never two writers inside one module.

## Stage 0 — foundation

Sequential, one careful owner, no fan-out
([12](./docs/12-development-process.md), *Build order*). Nothing else starts
until this is done, because everything else stands on it.

1. `go.mod`, and the repository skeleton of [09](./docs/09-code-layout.md).
2. `runtime/`: `Msg`/`Cmd`/`Meta`/`View`, module assembly (`runtime.New`),
   the reducer loop and inbound channel, the append-then-apply log with
   full-log replay, the built-in `send` command, subscription diffing — and
   **quiescence detection**, the subtlest concurrency in the project; it gets
   the most careful hands, not the fastest ([01](./docs/01-architecture.md)).
3. `store/`: SQLite open/schema for `message_log` (the rest of
   [03](./docs/03-persistence.md) lands with the modules that need it), with
   the in-memory twin.
4. `testlang/` and `kasi test`: parser/evaluator, the core vocabulary
   (`use`, `send`, the reads and verbs, `crash`/`restart`, `advance`), and
   the interpreter's own conformance corpus — the one sanctioned exception to
   "the scripts are the tests" ([14](./docs/14-test-language.md)).
5. The standing invariant checks in the runner: replay convergence,
   reducer-I/O detection, dead sends ([13](./docs/13-testing.md)); the
   secret-sentinel and archive-before-delete checks activate when secrets
   and workspaces exist.
6. The mechanical gates wired into CI or a pre-merge script: `kasi test t/`,
   `ast-grep scan`, reject `*_test.go`; the runner's cassette-provenance
   check (exercised from stage 2 on).

**Gate:** a hello scenario — a toy module, one `send`, model and command
assertions, a `crash`/`restart` — passes; `kasi test -n 100` of it passes;
`ast-grep scan` is clean.

## Stage 1 — the conversation, in memory

**Goal:** Flows A, B, and E of [10](./docs/10-flows.md) pass in the
simulation ring: a multi-turn email conversation with attachments in both
directions, the completion link, archive-then-delete, and
crash-at-the-worst-moment durability. This is the first priority and it is
not rushed; everything here is pure logic against simulated edges, so the
inner loop is milliseconds and there is no excuse for shortcuts.

In order:

1. **Contracts** (`kit manifest apply`, reviewed as the design act it is):
   `tasks/msg` (`create-task`, `append-to-task`, `finish-task`),
   `agents/msg` (`spawn-agent-run`, `stop-agent-run`), `email/msg`
   (`assemble-reply`, collaborator/allowlist messages) — exact payloads per
   [04](./docs/04-email.md), [05](./docs/05-agents-and-tasks.md),
   [10](./docs/10-flows.md).
2. **Flow scripts**, frozen after review: the invoice journey (Flow A, with
   `invoice.pdf` in and `receipt.pdf` out), the clarification loop (Flow B),
   crash-before-send and crash-mid-run (Flow E), and a fleet variant.
3. **`mime/`**: parse/build, part↔file lay-in and harvest
   ([02](./docs/02-object-model.md)). Fixtures are real RFC 5322 bytes —
   hand-captured from a real mailbox until ring 3 exists to record them.
4. **`email/`**: inbox/outbox slices, the allowlist, `route-email`
   (developed in partial assembly with the `dropped` read), reply assembly
   with threading headers, send + reconciliation against the sim mail edge.
5. **`tasks/`**: `create-task`/`append-to-task` handlers, workspace commands
   (create, lay-in, harvest, archive-then-delete), completion.
6. **`agents/`**: the harness interface, the **sim harness** (the first-class
   harness during development — [12](./docs/12-development-process.md)),
   agent-watch, the stop path. No vendor adapter yet.
7. Full assembly: the journey scenarios go green, then the fault-injection
   scenarios, then the fleet.

**Gate:** the invoice journey passes end-to-end in ring 1 — multi-turn, PDF
in, receipt out, reply-all threading, completion link, archive verified
before workspace deletion; the crash scenarios pass; `kasi test -n 100` of
the journey passes; `ast-grep scan` stays clean.

## Stage 2 — the conversation, for real

Turn the working logic into a working product by adding real edges — nothing
in the core changes ([13](./docs/13-testing.md), ring 3).

1. **`secrets/`** (minimal): the separate database, `secret://` resolution at
   the edge ([06](./docs/06-secrets.md)) — needed first for the Fastmail
   token.
2. **`email/` JMAP adapter** against Fastmail on a dedicated test domain
   ([04](./docs/04-email.md)).
3. **`agents/` first real harness adapter** (Claude today — an adapter, not
   the design target), validated by the harness conformance scripts; any
   later harness (pi.dev, our own) is the same interface and the same suite.
4. **Minimal web edge**: just enough HTTP to make capability links real —
   the completion link in an actual email must work ([04](./docs/04-email.md)).
   The rest of the web UI waits for stage 3.
5. **`tools/`** mise provisioning when the first template needs a real tool
   ([07](./docs/07-skills-and-tools.md)); defer if none does.
6. **Ring 3 probes and recording**: every live edge records; first cassettes
   committed with provenance; the recorded (ring 2) suite joins CI
   ([13](./docs/13-testing.md)).

**Gate:** on the test domain, for real: forward a mail with a PDF → a real
agent turn → a threaded reply → a human reply → a second turn with an
attachment back → completion link → archived. Every probe recorded; the
recorded suite runs green offline.

## Stage 3 — the web UI

Grow the UI on top of the working conversation, per [08](./docs/08-web-ui.md)
(htmlc + dispatch + Turbo). Each feature lands with a scenario driving the
web edge, and every view is scaffolded through the provider
(`kit generate kasi view.<name>`), which keeps the View-struct idiom uniform
([15](./docs/15-tactical-patterns.md)). In rough order:

1. Task list and task detail (read-only fallback view).
2. Live transcript view for a running agent; the Stop button.
3. **Agent-raised UI requests** — Flow C of [10](./docs/10-flows.md): the
   `request.json` contract, minting, the tokenised form, answers as
   references only.
4. Secrets management, route/template editing, allowlist editing.

**Gate:** Flow C passes in sim and recorded rings; the fallback view is
usable on a phone ([00](./docs/00-vision.md), principle 6).

## Stage 4 — supervision and skills

1. `control/` and the `kasi` control subcommands over the loopback socket
   ([11](./docs/11-supervisor.md)).
2. The supervisor route and template; its guard rails.
3. `skills/`: the registry, provisioning, and the agent-authored-skill loop —
   Flow D of [10](./docs/10-flows.md).

**Gate:** Flow D passes; in a live probe, a supervisor session lists, stops,
resumes, and archives tasks through the control interface.

## When something doesn't fit

If a stage surfaces a real design problem, the fix goes to the design docs
first, then to the contracts and scripts, then to the code — the drift rule
of [12](./docs/12-development-process.md) applied in the only order that
keeps the docs true.
