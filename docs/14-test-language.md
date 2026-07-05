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

That is the runtime's own shape — `update(model, msg) -> (model, cmds)`
([01](./01-architecture.md)) — so the language needs a handful of commands and
no abstraction facilities at all. There are no unit tests; these scripts are
the tests ([12](./12-development-process.md)).

## Blocks carry the structure

Tcl's one deep idea is that `{ … }` is an **unevaluated block** handed to a
command, and the command decides what the text inside means. The test language
leans on that everywhere: every command that takes structured input takes a
block and evaluates it line by line in its own small vocabulary — no JSON in
strings, no `-flags`, no escaping. A whole handler test is two blocks:

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

expect {
    commands            {create-workspace lay-in-inputs provision-workspace spawn-agent-run}
    task 1 route        pay
    task 1 participants {owner@example.test alice@example.test}
}
```

- In a **`send` block**, each line is `field value…` and the block builds the
  message's payload (serialised to JSON on the log, as always —
  [03](./03-persistence.md)). A braced value is a list; an omitted field is
  absent. The message then enters through the front of the runtime — logged,
  applied by the reducer — exactly as in production. Because messages are
  imperative and complete ([01](./01-architecture.md)), a script can hand-feed
  *any* sequence the real edges could produce, including the awkward ones
  they rarely do.
- In an **`expect` block**, each line is a **read followed by the expected
  value**: everything up to the last word is evaluated as a read, the last
  word is the want. A `~` before the want makes it a glob match for wording
  the test doesn't own: `outbound last body ~ "*confirm*"`.
- **`commands`** is a read like any other: it drains the trace of commands
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
- `#` starts a comment.

That is the complete grammar. There are **no procedures, no loops, no
conditionals, no includes**. A test is a straight line: stimulus, assertion,
stimulus, assertion. If a script seems to need abstraction, it is testing too
much — write two scripts. Repetition *across* scripts is accepted on purpose:
every script must read standalone, top to bottom, with no definitions to
chase.

## `use` — assembling the instance

`main.go` is the one place the real application is assembled: the full module
list, real edges ([01](./01-architecture.md), [09](./09-code-layout.md)). A
test script does the same thing, in miniature, as its first act:

```tcl
use email tasks agents    # a partial assembly: just these modules
use *                     # the full main.go assembly — the default
```

A script with no `use` line gets the whole application, which is right for
end-to-end scenarios. Naming modules gives a **partial assembly**, which is
right for driving a few domains' handlers directly — messages for absent
domains drop, exactly as the open-set rule promises
([01](./01-architecture.md)). Because instances are values with no global
registration, `use` is nothing more than construction — and `kasi test -n 100`
is just that construction performed a hundred times concurrently, which is
also what makes any hidden shared state fail loudly
([13](./13-testing.md)).

Modules bring their **test vocabulary** with them: the email module
contributes `deliver` and `outbound`, the agents module contributes `harness`,
and so on. Using a command from a module the script didn't assemble is an
error — a script states the world it lives in, then lives in it.

## Vocabulary

Small enough to list; the authoritative, current version is `kasi test help`.

| Command | Does |
|---------|------|
| `use <module…>` | Assemble the instance from these modules (default `*`) |
| `send <tag> { field value… }` | Feed one runtime message; the block builds the payload |
| `expect { <read…> <want> }` | Assert, one read per line; `~ <glob>` to pattern-match |
| `task`, `tasks`, `outbox`, `archive`, `skills`, `commands`, `command` | Reads over the model and the command trace |
| `settle` | Run until quiescent: channel empty, no effect in flight, no due timer ([13](./13-testing.md)) |
| `advance <duration>` | Move the virtual clock |
| `deliver { from … to … }` | Sim mail edge: store an inbox row, emit `route-email` — a delivery as production sees it ([04](./04-email.md)) |
| `outbound <last\|N> <field>` | Read mail the sim edge has sent: `to`, `subject`, `body`, `attachments`, `completion-link`, … |
| `click <url>` | Follow a capability link through the web edge ([04](./04-email.md)) |
| `harness { out <file> <content>… exit <code> }` | Queue the sim harness's next run ([05](./05-agents-and-tasks.md)) |
| `crash` / `restart` | Drop the model and goroutines, keeping only the log and content tables; replay and reconcile ([01](./01-architecture.md)) |
| `fail <edge> <op> [N]` | Make a simulated edge fail its next N operations (default 1) |
| `fixture <path>` | The bytes of a file under `t/fixtures/` |

`harness` is a queue, not a callback: each block queues one run, and each
`spawn-agent-run` pops one. A sim harness with an empty queue stays running
until signalled — which is exactly what a hung agent looks like, so the stop
path ([05](./05-agents-and-tasks.md)) needs no special support.

## Flow A, end to end

The full loop through the simulated edges — the executable counterpart of
Flow A in [10](./10-flows.md):

```tcl
# t/pay/invoice-confirmation.test — Flow A: confirm, then pay

harness {
    out reply.txt "Large amount - please confirm before I pay."
    exit 0
}

deliver {
    from    owner@example.test
    to      pay@kasi.test
    cc      alice@example.test
    subject "Invoice from Vendor X"
    attach  invoice.pdf [fixture pay/invoice.pdf]
}
settle

expect {
    task 1 status       awaiting-user
    outbound last to    {owner@example.test alice@example.test}
    outbound last body  ~ "*confirm*"
}

harness {
    out reply.txt   "Paid. Receipt attached."
    out receipt.pdf [fixture pay/receipt.pdf]
    exit 0
}

deliver {
    from  alice@example.test
    reply-to-last
    body  "yes, pay it"
}
settle

expect { outbound last attachments  receipt.pdf }

click [outbound last completion-link]
settle

expect {
    task 1 status                    done
    archive count task 1 transcript  2
}
```

Durability at an adversarial moment (Flow E of [10](./10-flows.md)):

```tcl
# t/runtime/crash-before-send.test — pending outbox survives a crash

harness { out reply.txt "done" ; exit 0 }
fail mail send

deliver { from owner@example.test ; to research@kasi.test ; subject "q" }
settle
expect { outbox last status  pending }

crash
restart
settle

expect {
    task 1 status       awaiting-user   # replay rebuilt the model
    outbox last status  sent            # reconciliation re-sent, exactly once
    outbound count      1
}
```

## The same script, ring by ring

`kasi test --ring=sim|recorded|live` selects the edge implementations
([13](./13-testing.md)); the script does not change:

- **sim** (default) — everything above, in memory, deterministic.
- **recorded** — the harness and mail edges replay the scenario's cassettes;
  `harness` blocks are ignored (the cassette *is* the agent's behaviour).
  Portable scripts assert outcomes and use `~` for wording reality owns.
- **live** — real edges; `harness` blocks are ignored; the run records,
  refreshing cassettes on success ([13](./13-testing.md)).

A script meant for one ring only declares it (`ring sim`, `ring live`) on its
first line.

## When a script fails

The runner's job on failure is to make the log tell the story:

- the failing `expect` line, with substitutions applied, *got*, and *want*;
- the tail of the instance's **message log** — tags and payloads, the complete
  record of what actually happened ([01](./01-architecture.md));
- any messages or commands **dropped as unhandled** — where a mistyped tag, or
  a module missing from `use`, surfaces;
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
knows nothing about email. The core vocabulary (`use`, `send`, `expect`,
`settle`, …) is registered by `kasi test`; each module contributes its own
edge commands alongside its simulated twins, which live in their domain
packages (`agents/harness_sim.go`, `email/…_sim.go`, …) beside their real
counterparts ([01](./01-architecture.md), [12](./12-development-process.md)).
