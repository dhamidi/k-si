# 14 — The test language

The test suite is a set of plain-text scripts under `t/` (extension `.test`),
run by the same single `kasi` binary that runs everything else:

```
kasi test t/                 # the whole suite
kasi test t/email/route.test # one script
```

The language is deliberately dumb. It borrows Tcl's surface — commands made of
whitespace-separated words, everything a string — and adds almost nothing,
because the loop it exists to express is small:

> **send a runtime message → assert on model fields and emitted commands.**

That is the shape of the runtime itself — `update(model, msg) -> (model, cmds)`
([01](./01-architecture.md)) — so the language needs exactly three load-bearing
commands (`send`, `expect`, and the reads they combine), plus a handful of
helpers for driving edges. There are no unit tests; these scripts are the tests
([12](./12-development-process.md)).

## The core loop

```tcl
# t/email/route-new-task.test — routing a fresh mail creates a task

send route-email {
  "inbox_id": 1,
  "recipient": "pay@kasi.test",
  "sender": "owner@example.test",
  "cc": ["alice@example.test"],
  "subject": "Invoice from Vendor X",
  "message_id": "<m1@example.test>",
  "in_reply_to": null
}

expect [commands] {create-workspace lay-in-inputs provision-workspace spawn-agent-run}
expect [model tasks 1 route] pay
expect [model tasks 1 participants] {owner@example.test alice@example.test}
```

- **`send <tag> <json>`** feeds one runtime message through the front of the
  runtime — logged, then applied by the reducer, exactly as in production.
  Because messages are imperative and complete ([01](./01-architecture.md)),
  a script can hand-feed *any* sequence the real edges could produce —
  including the awkward ones they rarely do.
- **`model <path…>`** reads a field of the in-RAM model by path.
- **`commands`** drains the trace of commands handlers have returned since it
  was last read, as a list of tags in order. The interpreter records every
  command before deciding whether to perform it, so the trace is assertable in
  every ring. **`command <tag> <path…>`** reads a payload field of the most
  recent command with that tag.
- **`expect <got> <want>`** compares strings; `expect -match <glob> <got>`
  pattern-matches for content whose exact wording the test doesn't own.

A script like the one above never runs an effect: it asserts on what the
handlers *returned*, which is the whole contract of the pure core. Whether
those commands are then executed depends on the ring
([13](./13-testing.md)) — the script text is the same either way.

## Grammar — all of it

- A script is a sequence of commands: **words separated by whitespace**, ended
  by newline or `;`. The first word is the command, the rest are arguments.
- **Everything is a string.** Lists are whitespace-separated words. JSON
  payloads are just brace-quoted strings handed to `send`.
- `$name` substitutes a variable; `set name value` assigns one.
- `[command …]` substitutes a command's result in place.
- `{ … }` quotes literally (no substitution); `" … "` quotes with
  substitution.
- `#` starts a comment.

That is the complete grammar. There are **no procedures, no loops, no
conditionals, no includes**. A test is a straight line: stimulus, assertion,
stimulus, assertion. If a script seems to need abstraction, it is testing too
much — write two scripts. Repetition *across* scripts is accepted on purpose:
every script must read standalone, top to bottom, with no definitions to chase.

## Driving the full loop

When a script should exercise effects too — the simulated edges interpreting
commands and feeding results back as messages
([13](./13-testing.md)) — a few edge commands provide stimulus and
observation. The full vocabulary is small enough to list; the authoritative,
current version is `kasi test help`:

| Command | Does |
|---------|------|
| `send <tag> <json>` | Feed one runtime message to the reducer |
| `model <path…>` | Read a model field |
| `commands` | Drain the command trace (tags, in order) |
| `command <tag> <path…>` | Read a payload field of the last such command |
| `expect [-match] <got> <want>` | Assert |
| `settle` | Run until quiescent: channel empty, no effect in flight, no due timer ([13](./13-testing.md)) |
| `advance <duration>` | Move the virtual clock |
| `deliver -from … -to … [-cc …] [-subject …] [-body …] [-attach …] [-raw …]` | Sim mail edge: store an inbox row, emit `route-email` — a delivery as production sees it ([04](./04-email.md)) |
| `outbound [last\|N] [<field>]` | Read mail the sim edge has sent: `to`, `subject`, `body`, `attachments`, `completion-link`, … |
| `click <url>` | Follow a capability link through the web edge ([04](./04-email.md)) |
| `harness next [-out <file> <content>]… [-exit <code>]` | Queue the sim harness's next run: the files it leaves in `out/` and its exit status ([05](./05-agents-and-tasks.md)) |
| `crash` / `restart` | Drop the model and goroutines, keeping only the log and content tables; replay and reconcile ([01](./01-architecture.md)) |
| `fail <edge> <op> [-times N]` | Make a simulated edge fail its next N operations |
| `fixture <path>` | The bytes of a file under `t/fixtures/` |

`harness next` is a queue, not a callback: each `spawn-agent-run` pops one
entry. A sim harness with an empty queue stays running until signalled — which
is exactly what a hung agent looks like, so the stop path
([05](./05-agents-and-tasks.md)) needs no special support.

Flow A of [10](./10-flows.md), end to end through the simulated edges:

```tcl
# t/pay/invoice-confirmation.test — Flow A: confirm, then pay

harness next -out reply.txt "Large amount - please confirm before I pay." -exit 0

deliver -from owner@example.test -to pay@kasi.test -cc alice@example.test \
        -subject "Invoice from Vendor X" -attach [fixture pay/invoice.pdf]
settle

expect [model tasks 1 status] awaiting-user
expect [outbound last to] {owner@example.test alice@example.test}
expect -match "*confirm*" [outbound last body]

harness next -out reply.txt "Paid. Receipt attached." \
             -out receipt.pdf [fixture pay/receipt.pdf] -exit 0

deliver -from alice@example.test -to pay@kasi.test -reply-to-last -body "yes, pay it"
settle

expect [outbound last attachments] receipt.pdf

click [outbound last completion-link]
settle

expect [model tasks 1 status] done
expect [model archive count -task 1 -kind transcript] 2
```

Durability at an adversarial moment (Flow E of [10](./10-flows.md)):

```tcl
# t/runtime/crash-before-send.test — pending outbox survives a crash

harness next -out reply.txt "done" -exit 0
fail mail send -times 1

deliver -from owner@example.test -to research@kasi.test -subject "q"
settle
expect [model outbox last status] pending

crash
restart
settle

expect [model tasks 1 status] awaiting-user   # replay rebuilt the model
expect [model outbox last status] sent        # reconciliation re-sent, exactly once
expect [outbound count] 1
```

## Fleets are a runner flag, not a language feature

Running the scale envelope ([12](./12-development-process.md)) needs no new
syntax: `kasi test -n 100 <script>` runs one hundred independent instances of
the same script concurrently, in one process. The language stays dumb; the
concurrency lives in the runner. Any cross-talk between instances (a stray
package-level variable) fails immediately.

## The same script, ring by ring

`kasi test --ring=sim|recorded|live` selects the edge implementations
([13](./13-testing.md)); the script does not change:

- **sim** (default) — everything above, in memory, deterministic.
- **recorded** — the harness and mail edges replay the scenario's cassettes;
  `harness next` is ignored (the cassette *is* the agent's behaviour).
  Portable scripts assert outcomes and use `-match` for wording reality owns.
- **live** — real edges; `harness next` is ignored; the run records,
  refreshing cassettes on success ([13](./13-testing.md)).

A script meant for one ring only declares it (`ring sim`, `ring live`) on its
first line.

## When a script fails

The runner's job on failure is to make the log tell the story:

- the failing `expect`, with substitutions applied, *got*, and *want*;
- the tail of the instance's **message log** — tags and payloads, the complete
  record of what actually happened ([01](./01-architecture.md));
- any messages or commands **dropped as unhandled** — where a mistyped tag,
  the known cost of the open-set design, surfaces;
- on recorded/live rings, the paths of the recordings involved.

Because the log plus the script reproduce the run exactly, "attach the failure
output" is a complete bug report.

## Where things live

Per the layout conventions of [09](./09-code-layout.md), and the single-binary
rule — there is one `kasi` executable; testing is a subcommand of it:

```
cmd/kasi/            # the single binary: serve, test, control subcommands
testlang/            # parser + evaluator for the script language, domain-agnostic
t/                   # test scripts (*.test), grouped by domain/flow
t/fixtures/          # real inputs (MIME, PDFs) captured or curated
t/cassettes/         # ring-2 recordings, refreshed by ring-3 probes
```

`testlang/` knows nothing about käsi — it parses and evaluates, mirroring how
`runtime/` knows nothing about email. The test vocabulary is registered by
`kasi test` and drives the edges' simulated twins, which live in their domain
packages (`agents/harness_sim.go`, `email/…_sim.go`, …) beside their real
counterparts ([12](./12-development-process.md)).
