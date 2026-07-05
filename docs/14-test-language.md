# 14 — The test language

Scenarios are written in **käsikiri** (Estonian for *manuscript* — the writing
that drives the hand), a small Tcl-inspired command language interpreted by
the scenario runner, `kasi-test`. Scripts live under `t/` with the `.kiri`
extension. There are no Go unit tests; these scripts are the test suite
([12](./12-development-process.md)).

## Why a language, and why Tcl-shaped

- **Tests are scenarios, not functions.** A käsi test is a story — mail
  arrives, an agent acts, a reply goes out, someone clicks done. A command
  language reads as that story; a `*_test.go` file reads as plumbing around
  it. The scripts in `t/` are the executable counterparts of the walkthroughs
  in [10](./10-flows.md).
- **One script, three rings.** Because scripts speak only in front-door terms
  (deliver mail, observe replies, advance time), the same scenario runs
  against simulated, recorded, or live edges ([13](./13-testing.md)). A test
  DSL bound to Go internals could never move between rings.
- **No compile step in the loop.** Editing a scenario and re-running is
  instant, which matters when the inner loop runs every few seconds
  ([12](./12-development-process.md)).
- **Agents can write tests.** A tiny, regular, textual language is exactly
  what an agent — including käsi's own — can author and read reliably. The
  test suite is a corpus agents can extend.
- **Tcl's shape is the right shape.** Tcl is the minimal language where
  *everything is a command and everything is a string*: trivial to interpret,
  trivial to extend with domain commands, impossible to over-abstract. We
  implement the small core ourselves — on the order of a few hundred lines of
  Go, in keeping with the dependency-light rule ([00](./00-vision.md)) — we do
  not embed a full Tcl.

## The language core

The entire grammar, in brief. If you know Tcl, you know this; if not, five
minutes suffices:

- A script is a sequence of **commands**. A command is **words separated by
  whitespace**, terminated by newline or `;`. The first word names the
  command; the rest are its arguments.
- **Everything is a string.** Numbers, lists, and handles are strings with
  conventions (lists are whitespace-separated words).
- `$name` substitutes a **variable**; `set name value` assigns one.
- `[command …]` substitutes the **result of running a command** in place.
- `{ … }` **quotes without substitution** — used for blocks passed to control
  commands and for literal text. `" … "` quotes with substitution.
- `#` begins a comment.
- Control flow is not syntax, just commands taking blocks: `if`, `foreach`,
  `while`, `proc` (define a procedure). A scenario needing more than these is
  probably testing too much at once.

That is the whole language. Everything else is **vocabulary**: commands
registered by the runner, in an open registry keyed by name — the same
open-set philosophy as the runtime's message registry
([01](./01-architecture.md)). Adding a capability to the test harness means
registering a command; there is no grammar to extend.

## Vocabulary

The vocabulary mirrors the domain packages ([09](./09-code-layout.md)):
commands are grouped by the edge or domain they drive. The authoritative,
always-current list is `kasi-test help`; the table below shows the shape of
each group, not an exhaustive inventory.

| Group | Representative commands | Drives |
|-------|------------------------|--------|
| Instance | `instance`, `settle`, `crash`, `restart`, `fleet N { … }` | The runtime lifecycle ([01](./01-architecture.md)) |
| Mail | `mail deliver`, `mail reply`, `mail outbound`, `click` | The mail edge, in both directions ([04](./04-email.md)) |
| Agent | `agent on-turn N { … }`, `reply`, `artifact`, `ask`, `hang` | The scripted harness ([05](./05-agents-and-tasks.md)) |
| Clock | `clock advance 5m` | Virtual time ([13](./13-testing.md)) |
| Reads | `task`, `tasks`, `archive`, `skills` | The model and content tables, read-only |
| Assertions | `expect`, `expect -match` | — |
| Faults | `fail mail send -times 2`, … | Edge fault injection ([13](./13-testing.md)) |
| Recordings | `fixture`, `cassette` | Ring-2 material ([13](./13-testing.md)) |

Conventions that keep scripts uniform:

- **Reads are commands returning strings**, composed via substitution:
  `expect [task 1 status] awaiting-user`.
- **Injections are imperative commands** whose effect is to feed the instance
  through its front door — `mail deliver` ends up as a `route-email` message
  exactly as a real delivery would, via the simulated edge, never by poking
  the model.
- **`settle` after every stimulus.** Stimulus, settle, assert — the universal
  rhythm ([13](./13-testing.md)).

## A scenario, end to end

Flow A of [10](./10-flows.md) — pay an invoice, with a confirmation round-trip
— as a scenario. Prose walkthrough and script are two notations for the same
specification:

```tcl
# t/pay/invoice-confirmation.kiri — Flow A: confirm, then pay

instance

agent on-turn 1 {
    ask "This invoice is for a large amount - please confirm before I pay."
}
agent on-turn 2 {
    reply "Paid. Receipt attached."
    artifact receipt.pdf [fixture pay/receipt.pdf]
}

mail deliver -from owner@example.test -to pay@kasi.test \
             -cc alice@example.test \
             -subject "Invoice from Vendor X" \
             -attach [fixture pay/invoice.pdf]
settle

expect [task 1 status]   awaiting-user
expect [task 1 participants] {owner@example.test alice@example.test}
expect [mail outbound last to] {owner@example.test alice@example.test}
expect -match "*confirm*" [mail outbound last body]

mail reply -from alice@example.test -body "yes, pay it"
settle

expect [task 1 status] awaiting-user
expect [mail outbound last attachments] receipt.pdf

click [mail outbound last completion-link]
settle

expect [task 1 status] done
expect [archive count -task 1 -kind transcript] 2
```

Durability, tested at an adversarial moment (Flow E of [10](./10-flows.md)):

```tcl
# t/runtime/crash-before-send.kiri — pending outbox survives a crash

instance
agent on-turn 1 { reply "done" }
fail mail send -times 1                 ;# the first send attempt will fail

mail deliver -from owner@example.test -to research@kasi.test -subject "q"
settle
expect [mail outbound last status] pending

crash                                    ;# drop model + goroutines; keep log + tables
restart                                  ;# full-log replay, then reconciliation
settle

expect [task 1 status] awaiting-user     ;# replay rebuilt the model
expect [mail outbound last status] sent  ;# reconciliation re-sent, exactly once
expect [mail outbound count] 1
```

And the scale envelope, proved in one process ([12](./12-development-process.md)):

```tcl
# t/runtime/fleet.kiri — a hundred instances, concurrently, one process

fleet 100 {
    agent on-turn 1 { reply "done: $I" }          ;# $I = this instance's index
    mail deliver -from owner@example.test -to pay@kasi.test -subject "job $I"
    settle
    expect [task 1 status] awaiting-user
}
```

## The same script, ring by ring

`kasi-test --ring=sim|recorded|live t/…` selects the edge implementations
([13](./13-testing.md)); the script does not change. What each ring does with
the script:

- **sim** — `agent on-turn` blocks program the scripted harness; mail is
  in-memory; the run is total-deterministic.
- **recorded** — the harness and mail edges replay the scenario's cassettes;
  `agent on-turn` blocks are ignored (the cassette *is* the agent's
  behaviour). Assertions still run — which is why portable scenarios assert
  outcomes and use `-match` for content whose wording reality owns.
- **live** — real edges; `agent on-turn` blocks are ignored; the run records,
  refreshing cassettes on success ([13](./13-testing.md)).

A scenario meant for one ring only says so (`ring sim`, `ring live`) at the
top; most scenarios are portable by simply following the assert-on-outcomes
convention.

## When a scenario fails

The runner's job on failure is to make the log tell the story
([01](./01-architecture.md)):

- the failing `expect`, with the substituted command, *got*, and *want*;
- the tail of the instance's **message log** — tags and payloads, the
  complete record of what actually happened;
- any messages **dropped as unhandled**, which is where a mistyped tag —
  the known cost of the open-set design — surfaces;
- on ring 2/3, the paths of the recordings involved.

Because the log plus the scenario reproduce the run exactly, "attach the
failure output" is a complete bug report.

## Where things live

Per the layout conventions of [09](./09-code-layout.md):

```
cmd/kasi-test/       # the runner: interpreter + vocabulary + ring wiring
kiri/                # the language core: parser + evaluator, domain-agnostic
t/                   # scenario scripts (*.kiri), grouped by domain/flow
t/fixtures/          # real inputs (MIME, PDFs) captured or curated
t/cassettes/         # ring-2 recordings, refreshed by ring-3 probes
```

`kiri/` knows nothing about käsi, mirroring how `runtime/` knows nothing about
email; the vocabulary lives with the runner and drives the edges' simulated
twins, which live in their domain packages (`agents/harness_sim.go`,
`email/…_sim.go`, …) beside their real counterparts
([12](./12-development-process.md)).
