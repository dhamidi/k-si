# 12 — Development process

This document describes *how käsi is built*: the feedback loop, the rules that
keep that loop fast, and where a new developer starts. It is deliberately
evergreen — it states the invariants of the process, not the current state of
the code, so it should read true regardless of which features exist yet.

If you are new to the project, this is your second stop. Read
[00](./00-vision.md) for what käsi is, skim [01](./01-architecture.md) for the
runtime it is built on, then come back here. Everything else can be read on
demand.

## The mental model in five sentences

1. käsi is one Go process whose entire state is an in-RAM **model**, computed as
   a pure fold of an append-only **message log** ([01](./01-architecture.md)).
2. **Handlers** are pure: `(model, msg) -> (model, cmds)`. They never touch the
   world.
3. All I/O lives at the **edges** — command effects and subscriptions — which
   talk to the world and feed results back in as complete, imperative messages.
4. Because of 1–3, the whole application logic can run **without the world**:
   swap the edges for simulated ones and the entire system — routing, tasks,
   agent runs, replies, archival — executes in memory, deterministically, in
   microseconds.
5. Tests are therefore **scenarios driven through the front door** (mail in,
   replies out), written in a small Tcl-style scripting language
   ([14](./14-test-language.md)) and run against simulated, recorded, or live
   edges ([13](./13-testing.md)) — never unit tests against internals.

Sentence 4 is the engine of the whole development process. Everything below
exists to protect it.

## Finding your bearings

The suggested path from zero to productive:

1. **Read [00](./00-vision.md)** — vision, principles, glossary. The glossary
   matters: the docs use its terms precisely.
2. **Skim [01](./01-architecture.md)** — the runtime. You need the
   message/command/subscription cycle and the replay story in your head.
3. **Read this document**, then [13](./13-testing.md) (the three test rings)
   and [14](./14-test-language.md) (the test language).
4. **Run the scenario suite** (`kasi test t/`) and watch it pass in well under a
   second. That speed is the point.
5. **Open a scenario script next to its flow.** The walkthroughs in
   [10](./10-flows.md) and the scripts in `t/` describe the same happy paths in
   two notations — prose and executable. Reading them side by side teaches you
   both the system and the test language at once.
6. **Write a scenario** for something small before writing any Go. If you can
   express the behaviour you want as a script, you understand the design; if
   you can't, you've found the question to ask.

The remaining design docs ([02](./02-object-model.md)–[11](./11-supervisor.md))
are references: read each one when you first touch its domain.

## The loop

The inner development loop is:

> **edit → `kasi test t/` → repeat**, on the order of milliseconds per cycle.

The scenario suite runs the *entire* system — every domain, wired together,
end-to-end — against simulated edges in a single process
([13](./13-testing.md)). There is no deployment, no sandbox account, no live
agent, and no sleeping in the loop. Feedback is immediate, so the loop is run
constantly, not as a pre-commit chore.

Live end-to-end probes — really spawning a harness, really sending mail —
exist, but they are the *outer* loop: rare, scheduled, and run when touching an
edge or preparing a release ([13](./13-testing.md)). The inner loop must never
depend on them.

For a new capability, work usually flows in this order:

1. **Design first.** If the capability changes an invariant or adds a concept,
   the design docs change first (see *Docs discipline* below). Most features
   don't; they fit the existing shape.
2. **Scenario first.** Write the script that demonstrates the behaviour — the
   executable counterpart of a [10](./10-flows.md) walkthrough. It fails.
3. **Messages and handlers.** Add the imperative messages, their pure handlers,
   and the commands they return, in the owning domain package
   ([09](./09-code-layout.md)).
4. **Edges last.** If the capability needs new I/O, add the command effect or
   subscription — *together with its simulated twin* (next section).
5. The scenario passes in the simulation ring. If a new edge was involved, a
   live probe exercises it for real, and its recording joins the deterministic
   suite ([13](./13-testing.md)).

## The seam rule

The one rule that keeps the in-memory story true forever:

> **Every edge ships with a simulated twin.** No command effect and no
> subscription may land without an in-memory implementation that the scenario
> runner can drive and observe. A real edge without a sim twin is an
> unfinished edge.

Edges are the *only* places käsi touches the world, and each is defined by a
small interface with (at least) two implementations, living side by side per
the naming conventions of [09](./09-code-layout.md) (e.g. `harness_claude.go`
beside `harness_sim.go`):

| Edge | Real implementation | Simulated twin |
|------|--------------------|----------------|
| Mail provider | JMAP against Fastmail ([04](./04-email.md)) | In-memory mailbox: scripts inject inbound mail, observe outbound |
| Agent harness | Official harness CLI/SDK ([05](./05-agents-and-tasks.md)) | Scripted agent: the scenario declares what the "agent" does each turn |
| Clock | Wall clock ticker | Virtual clock advanced explicitly by the script |
| Store | SQLite files ([03](./03-persistence.md)) | SQLite in-memory (same engine, same SQL, no disk) |
| Workspace | `$WORKDIR` on disk | Per-instance temp or in-memory tree |
| Tools/skills provisioning | mise ([07](./07-skills-and-tools.md)) | No-op recorder: asserts *what would be* provisioned |
| Secrets | Secrets DB + resolver ([06](./06-secrets.md)) | In-memory secrets with sentinel values (used to detect leaks — [13](./13-testing.md)) |
| Control interface | Loopback socket ([11](./11-supervisor.md)) | Direct in-process calls |

This table is illustrative, not exhaustive — the rule applies to any edge that
exists now or later. The discipline is cheap at the moment an edge is written
(the sim twin is usually a few dozen lines) and unaffordable to retrofit once a
body of logic depends on an unsimulatable edge.

The runtime's own design enforces the other half of the seam: handlers cannot
need simulation because they do no I/O at all. If you find yourself wanting to
simulate something *inside* a handler, the I/O is on the wrong side of the seam
— move it to an edge and deliver its result as a message
([01](./01-architecture.md)).

## The scale envelope

käsi is small on purpose, and the numbers are part of the design
([00](./00-vision.md)):

- **At most ~20 people** ever interact with an instance — the owner plus
  participants CC'd into threads — with only a **handful actively messaging**
  at any moment.
- **~100 concurrent agent runs** is the load the system must handle
  comfortably: many slow, long-running worker processes against a trickle of
  human traffic.

Implications for development:

- **Do not build for scale beyond the envelope.** No sharding, no worker
  fleets, no caching layers. One process, business objects in RAM, full-log
  replay — the envelope is what makes these simplifications correct.
- **Do prove the envelope, continuously.** The simulation ring runs *fleets* of
  in-memory instances — on the order of a hundred in one test process — each
  juggling multiple concurrent (simulated) agent runs, which places the tested
  load beyond the real one ([13](./13-testing.md)). Headroom is demonstrated,
  not assumed.
- The interesting axis is **concurrent agent runs**, not requests per second.
  Contention worth testing lives around the single reducer, the subscription
  diff, and many simultaneous `finish-agent-run` messages — not in HTTP
  throughput.

## No unit tests

käsi deliberately has **no unit tests** and does not use Go's `testing` package
for behaviour. The repository's tests are scripts under `t/`, run by
`kasi test` ([14](./14-test-language.md)).

Why this is safe here, when it wouldn't be elsewhere:

- **The unit is the system.** Because the whole system runs in memory in
  microseconds, testing through the front door costs the same as testing a
  function — so we buy the integration coverage for free and skip the mock
  scaffolding entirely.
- **Purity removes the usual reason for unit tests.** A handler is
  `(model, msg) -> (model, cmds)`; the scenario that delivers the message and
  observes the consequences *is* the handler's test, with the added value that
  the message's shape, the registry wiring, and the downstream commands are
  tested too.
- **The failure story stays good.** When a scenario fails, the instance's
  message log — the complete, replayable record of everything that happened —
  is the debugging artifact ([13](./13-testing.md)). There is no "which mock
  was mis-set-up" archaeology.

If you feel the urge to unit-test something, treat it as a design signal:

- *A handler feels too complex to reach through scenarios* → the handler is too
  big; split the message or move decisions into the model
  ([01](./01-architecture.md)).
- *Pure logic buried in an edge needs testing* → the logic doesn't belong in
  the edge; lift it into a handler or a pure helper the handlers use, where
  scenarios reach it naturally.
- *A tricky pure algorithm (MIME part mapping, threading keys) needs many
  cases* → write them as a straight-line script, one send/expect pair per
  case; real captured MIME from live probes makes better cases than
  hand-built ones ([13](./13-testing.md)).

## Harness-agnostic by construction

käsi shells out to official agent harnesses behind a thin interface
([05](./05-agents-and-tasks.md)). Claude is the *default adapter*, chosen for
convenience — it is not the design target, and more harnesses will follow.

The development consequences:

- **Code against the interface, never the adapter.** Nothing outside a
  `harness_*.go` adapter may know which harness is in use. If a feature needs a
  capability the interface lacks, extend the interface, then implement it per
  adapter.
- **The simulated harness is the first-class harness during development.** All
  logic — lifecycle, transcripts, stopping, resumption, `out/` harvesting — is
  developed and verified against it. A real adapter adds no logic; it only
  translates the interface to one vendor's CLI.
- **Adapters are validated by a conformance suite**: a shared set of scenario
  scripts that any adapter must pass in the live ring — start, resume, produce
  output, get stopped, leave a locatable transcript ([13](./13-testing.md)).
  Adding a new harness (a different vendor's CLI, or eventually käsi's own) is:
  implement the interface, pass the conformance suite. No runtime changes, no
  test rewrites.

## Docs discipline

The design documents (this one included) are **evergreen**: they state the
shape of the system and the invariants it holds, not its current feature list
or file inventory. That has two operational consequences:

- **Drift is a bug.** When observed behaviour and a design doc disagree, one of
  them is wrong — fix whichever one it is, in the same change. Do not let the
  docs become historical fiction.
- **Update docs when invariants change, not when features land.** Adding a
  route, a skill, or a scenario needs no doc change. Changing what a task *is*,
  what the log guarantees, or how edges work does.

The division of labour among artifacts:

| Artifact | States | Changes when |
|----------|--------|--------------|
| Design docs (`docs/`) | Invariants, shape, reasons | An invariant changes |
| Scenario scripts (`t/`) | Concrete behaviour, executable | Behaviour is added or changed |
| Recordings/fixtures (`t/cassettes/`, `t/fixtures/`) | What the real world actually said | A live probe refreshes them |
| Code | The implementation | Constantly |

Scenarios are where the churn goes. Because they are executable, they cannot
drift silently the way prose can — which is exactly why the docs can afford to
stay still.
