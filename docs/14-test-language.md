# 14 — The test language

The test suite is a set of plain-text scripts under `t/` (extension `.test`),
run by the same single `kasi` binary that runs everything else:

```
kasi test t/                    # the whole suite
kasi test t/email/route.test    # one script
kasi test -n 100 <script>       # a fleet: 100 instances of it, one process
```

The language is deliberately dumb. It is Tcl's surface — commands made of
whitespace-separated words, everything a string, blocks in braces — and
nothing else, because the loop it exists to express is small:

> **assemble an instance from modules → send runtime messages → assert on
> model fields and emitted commands.**

There are no unit tests; these scripts are the tests
([12](./12-development-process.md)). And a script is written to read as a
**user journey**: a chronological story — mail arrives, the agent works, a
reply goes out, someone clicks done — in which every line is either something
that *happens* or something that *should now be true*.

## A script is a journey

Flow A of [10](./10-flows.md), as its executable counterpart:

```tcl
# t/pay/invoice-confirmation.test — Flow A: confirm, then pay

# You forward an invoice to pay@, CCing your accountant.
deliver {
    from    owner@example.test
    to      pay@kasi.test
    cc      alice@example.test
    subject "Invoice from Vendor X"
    attach  invoice.pdf [fixture pay/invoice.pdf]
}

task 1 status is awaiting-agent

# The agent sees the amount is large and asks before paying.
agent {
    out reply.txt "Large amount - please confirm before I pay."
}

task 1 status is awaiting-user
outbound last to is {owner@example.test alice@example.test}
outbound last body matches "*confirm*"

# Alice confirms, in the same thread.
deliver {
    from alice@example.test
    reply-to-last
    body "yes, pay it"
}

# The agent resumes its session, pays, and attaches the receipt.
agent {
    out reply.txt   "Paid. Receipt attached."
    out receipt.pdf [fixture pay/receipt.pdf]
}

outbound last attachments is receipt.pdf

# A participant clicks the completion link.
click [outbound last completion-link]

task 1 status is done
archive count task 1 transcript is 2
```

Three rules produce this shape:

1. **Stimuli settle themselves.** Every command that puts something in motion
   (`deliver`, `send`, `agent`, `click`, `restart`, `advance`) runs the
   instance to **quiescence** before returning — inbound channel empty, no
   effect in flight, no virtual timer due ([13](./13-testing.md)). There is
   no `settle` command and no sleeping: when the next line runs, the world
   has finished reacting to the previous one. An agent run waiting for its
   `agent` block is a *stable* state, not pending work — so "the agent is
   still working" is a perfectly good place for a script to stand.

2. **Every read is also an assertion.** `task 1 status` is a read; add a verb
   and a value and it asserts: `task 1 status is awaiting-user`. The verbs
   are `is` (string equality; `are` is a synonym that reads better for
   lists) and `matches` (glob, for wording the test doesn't own). Each
   expectation is its own sentence at the moment in the story where it should
   hold — there is no `expect` block bundling them up after the fact.

3. **The agent's turn happens where it happens in the story.** `agent { … }`
   completes the currently running (or next spawned) simulated agent run:
   the files it leaves in `out/`, and optionally `exit <code>` (default 0).
   It is written *after* the mail that triggers the run, in chronological
   order — the script never has to pre-program the future.

Narration is comments. The runner treats the nearest comment above a failing
line as the step name in its report, so the story structure is also the
failure structure — no `step`/`scenario` scaffolding needed.

## Blocks carry the structure

Tcl's one deep idea is that `{ … }` is an **unevaluated block** handed to a
command, and the command decides what the text inside means. Every command
that takes structured input takes a block and evaluates it line by line in
its own small vocabulary — no JSON in strings, no `-flags`, no escaping.
`deliver` and `agent` above are this; so is `send`, the bottom-most stimulus,
which builds a runtime message payload field by field:

```tcl
# t/email/route-new-task.test — routing a fresh mail creates a task
use email tasks

send route-email {
    inbox_id    1
    recipient   pay@kasi.test
    sender      owner@example.test
    cc          {alice@example.test}
    subject     "Invoice from Vendor X"
    message_id  <m1@example.test>
}

commands are {send:create-task create-workspace lay-in-inputs provision-workspace spawn-agent-run}
task 1 route is pay
task 1 participants are {owner@example.test alice@example.test}
```

A `send` command ([01](./01-architecture.md)) appears in the trace as
`send:<tag>` of the message it carries — the cross-domain hop is part of the
observable story, not plumbing hidden from it.

- In a `send` block, each line is `field value…`; the payload is serialised
  to JSON on the log as always ([03](./03-persistence.md)). A braced value is
  a list; an omitted field is absent. The runner decodes the block **against
  the tag's registered payload struct** ([15](./15-tactical-patterns.md)) and
  a field the struct doesn't have fails the script — production tolerates
  drift for old logs' sake; tests never do. The message then enters through
  the front of the runtime — logged, applied by the reducer — exactly as in
  production.
  Because messages are imperative and complete ([01](./01-architecture.md)),
  a script can hand-feed *any* sequence the real edges could produce,
  including the awkward ones they rarely do.
- `commands` is a read like any other: it drains the trace of commands
  handlers have returned since it was last read, as a list of tags in order.
  The interpreter records every command before deciding whether to perform
  it, so the trace is assertable in every ring ([13](./13-testing.md)).
  `command <tag> <field>` reads a payload field of the most recent command
  with that tag.

Note the symmetry with the runtime itself: braces give a block no meaning of
its own — the receiving command supplies it — just as a payload means nothing
until its tag's handler decodes it ([01](./01-architecture.md)).

## Grammar — all of it

- A script is a sequence of commands: **words separated by whitespace**, ended
  by newline or `;`. The first word is the command, the rest are arguments.
- **Everything is a string.** Lists are whitespace-separated words.
- `$name` substitutes a variable; `set name value` assigns one.
- `[command …]` substitutes a command's result in place.
- `{ … }` quotes a block literally — no substitution until (and unless) the
  receiving command evaluates it, in its own vocabulary, where `[ … ]` and
  `$` work again. `" … "` quotes with substitution.
- `#` starts a comment — and doubles as the story's narration.

That is the complete grammar. There are **no procedures, no loops, no
conditionals, no includes**. A test is a straight line: things happen, truths
are stated. If a script seems to need abstraction, it is testing too much —
write two scripts. Repetition *across* scripts is accepted on purpose: every
script must read standalone, top to bottom, like the journey it describes.

## `use` — assembling the instance

`main.go` is the one place the real application is assembled: the full module
list, real edges ([01](./01-architecture.md), [09](./09-code-layout.md)). A
test script does the same thing, in miniature, as its first act:

```tcl
use email tasks agents    # a partial assembly: just these modules
use *                     # the full main.go assembly — the default
```

A script with no `use` line gets the whole application, which is right for
journeys. Naming modules gives a **partial assembly**, right for driving a
few domains' handlers directly — messages for absent domains drop, exactly as
the open-set rule promises ([01](./01-architecture.md)), and the `dropped`
read returns them, so the boundary itself is assertable: assemble only
`email`, route a mail, and `dropped is create-task` states the hand-off
without needing `tasks/` to exist yet. That is how a domain gets built and
tested against nothing but its agreed tags ([12](./12-development-process.md)).
In a **full** assembly the same drop *fails* the script ([13](./13-testing.md)):
a complete build aiming a message at nothing is always a bug.

Because instances are values with no global registration, `use` is nothing
more than construction — and `kasi test -n 100` is that construction
performed a hundred times concurrently, which is also what makes any hidden
shared state fail loudly ([13](./13-testing.md)).

Modules bring their **test vocabulary** with them: the email module
contributes `deliver` and `outbound`, the agents module contributes `agent`,
and so on. Using a command from a module the script didn't assemble is an
error — a script states the world it lives in, then lives in it.

## Vocabulary

Small enough to list; the authoritative, current version is `kasi test help`.

**Things that happen** (each settles the instance before returning):

| Command | Does |
|---------|------|
| `deliver { from … to … }` | Mail arrives: the sim mail edge stores an inbox row and emits `route-email`, as production would ([04](./04-email.md)) |
| `agent { out <file> <content>… [exit <code>] }` | The running agent turn completes with these outputs ([05](./05-agents-and-tasks.md)) |
| `send <tag> { field value… }` | One runtime message enters the reducer; the block builds the payload |
| `click <url>` | A capability link is followed through the web edge ([04](./04-email.md)) |
| `advance <duration>` | The virtual clock moves |
| `crash` / `restart` | The process dies (model and goroutines gone; log and content tables kept) / comes back: full replay, then reconciliation ([01](./01-architecture.md)) |
| `fail <edge> <op> [N]` | The next N operations on a simulated edge will fail (default 1) |

**Things that should be true** — reads, which assert when given a verb
(`is`/`are` for equality, `matches` for a glob), and substitute their value
inside `[ … ]` otherwise:

| Read | Over |
|------|------|
| `task <id> <field…>`, `tasks <field…>` | The model's tasks |
| `outbox <last\|N> <field>`, `archive <…>`, `skills <…>` | The model and content tables |
| `outbound <last\|N\|count> [<field>]` | Mail the sim edge has sent: `to`, `subject`, `body`, `attachments`, `completion-link`, … |
| `commands`, `command <tag> <field>` | The drained command trace (`send` renders as `send:<tag>`) |
| `dropped` | Messages sent but unhandled in this assembly — expected at a partial assembly's boundary, fatal in a full one ([13](./13-testing.md)) |

**Setup**: `use <module…>` assembles the instance; `fixture <path>` reads
bytes from `t/fixtures/`; `set` assigns a variable; `ring sim|live` pins a
script to one ring.

An agent run whose `agent` block hasn't arrived yet simply keeps "running" —
which is exactly what a hung agent looks like, so the stop path
([05](./05-agents-and-tasks.md)) is tested by sending `stop-agent-run` while
the script stands in that state, no special support needed.

## Durability, as a journey

Flow E of [10](./10-flows.md) — a crash at the worst moment:

```tcl
# t/runtime/crash-before-send.test — a pending reply survives a crash

deliver { from owner@example.test ; to research@kasi.test ; subject "q" }

# The send will fail on its first attempt...
fail mail send

# ...so when the agent finishes, the reply gets queued but not sent.
agent { out reply.txt "done" }
outbox last status is pending

# The process dies and comes back: full-log replay, then reconciliation.
crash
restart

task 1 status is awaiting-user      # replay rebuilt the model
outbox last status is sent          # reconciliation re-sent it
outbound count is 1                 # exactly once
```

## The same script, ring by ring

`kasi test --ring=sim|recorded|live` selects the edge implementations
([13](./13-testing.md)); the script does not change:

- **sim** (default) — everything above, in memory, deterministic.
- **recorded** — the harness and mail edges replay the scenario's cassettes.
  An `agent` block's *contents* are ignored (the cassette is the agent's
  behaviour), but the command still marks where the turn completes, so the
  journey's rhythm is preserved. Portable scripts assert outcomes and use
  `matches` for wording reality owns.
- **live** — real edges. `agent` becomes "wait for the real agent's turn to
  finish"; the run records, refreshing cassettes on success
  ([13](./13-testing.md)).

## When a script fails

The runner's job on failure is to make the story tell itself:

- the failing sentence, with substitutions applied, *got*, and *want* — under
  the **narration comment** it falls under, so the report reads "in 'Alice
  confirms, in the same thread': `task 1 status` is `awaiting-agent`, wanted
  `awaiting-user`";
- the tail of the instance's **message log** — tags and payloads, the
  complete record of what actually happened ([01](./01-architecture.md));
- any messages or commands **dropped as unhandled** — where a mistyped tag,
  or a module missing from `use`, surfaces;
- on recorded/live rings, the paths of the recordings involved.

Because the log plus the script reproduce the run exactly, "attach the failure
output" is a complete bug report.

## Where things live

Per the layout conventions of [09](./09-code-layout.md), and the single-binary
rule — one `kasi` executable; testing is a subcommand of it:

```
cmd/kasi/            # main.go: the assembly point; serve, test, control subcommands
testlang/            # parser + evaluator for the script language, domain-agnostic
t/                   # test scripts (*.test), grouped by domain/flow
t/fixtures/          # real inputs (MIME, PDFs) captured or curated
t/cassettes/         # ring-2 recordings, refreshed by ring-3 probes
```

`testlang/` knows nothing about käsi — it parses words, blocks, and
substitutions, and hands commands to a vocabulary, mirroring how `runtime/`
knows nothing about email. The core vocabulary (`use`, `send`, the reads and
verbs) is registered by `kasi test`; each module contributes its own
commands (`deliver`, `agent`, …) alongside its simulated twins, which live in
their domain packages (`agents/harness_sim.go`, `email/…_sim.go`, …) beside
their real counterparts ([01](./01-architecture.md),
[12](./12-development-process.md)).
