# 01 — Runtime architecture

The core of käsi is an Elm-Architecture (TEA) runtime with one deliberate
twist: the set of message and command types is **open**, and anything the
running build does not recognise is dropped rather than rejected. This document
defines the runtime, its concurrency model, and how the whole application state
is made durable by logging and replaying messages.

## The Elm Architecture, restated

Three pure things and one impure loop:

- **Model** — the entire application state, in RAM.
- **update** — `update(model, msg) -> (model, []Cmd)`. A pure function. Given the
  current model and one runtime message, it returns the next model and a list of
  commands to run. It performs no I/O.
- **Cmd** — a *value* describing an effect ("send this email", "spawn this agent
  run", "write this row"). Handlers return commands; they never execute them.
- **subscriptions** — `subscriptions(model) -> []Sub`. A pure function of the
  model declaring which long-lived message sources should currently be alive.

The impure loop — the **runtime** — closes the cycle: it feeds messages to
`update`, interprets the returned commands into real effects, and turns the
results of those effects back into messages that re-enter `update`. It also
diffs the declared subscriptions against the running ones and starts/stops
sources accordingly.

```
        ┌──────────────────────────────────────────────┐
        │                                              │
   msg  ▼                                              │ msg
  ────────────►  update(model,msg) ──► (model, cmds)   │
                      ▲                     │           │
                      │                     ▼           │
                 subscriptions        command           │
                      │              interpreter ───────┘
                      │                (effects)
                      ▼
                 sources (pollers, timers, streams)
```

## The twist: open message and command sets

In classic Elm, `Msg` is a closed sum type; the compiler forces `update` to
handle every case. käsi does the opposite on purpose.

- A **runtime message** is identified by a stable string **type tag** (e.g.
  `route-email`, `finish-agent-run`, `mark-email-sent`) and carries a payload.
- Handlers are registered in a map keyed by type tag. Dispatch looks up the
  handler for the incoming message's tag.
- **If no handler is registered, the message is dropped** — recorded as
  unhandled, but otherwise a no-op. Same for commands: an unknown command tag is
  dropped by the interpreter.

Why:

- **No central union to edit.** A new capability registers its own handlers and
  command interpreters in its own package (see [09](./09-code-layout.md)). There
  is no god-file listing every message type.
- **Forward/backward log compatibility.** An old log may contain messages a new
  build no longer handles, or a new build may encounter tags an older one never
  emitted. Dropping unknowns means logs stay replayable across builds instead of
  crashing replay.
- **Graceful degradation.** Disabling a feature is as simple as not registering
  its handler; its messages become inert.

The cost is that a typo in a tag fails silently. We mitigate this with: a single
registry per domain, tests that assert the expected tags are registered, and a
startup log line enumerating every registered tag.

## Message discipline: imperative and complete

Two rules govern every runtime message, without exception. They are what make
handlers pure and replay deterministic.

1. **Imperative mood, always.** A message's tag is phrased as a command to the
   model — `route-email`, `finish-agent-run`, `mark-email-sent`, `finish-task`,
   `register-skill` — never as a passive event (`email.received`,
   `agent.finished`). The message *instructs the model to make a state change*.
   This reads naturally because a message is, precisely, an order the reducer
   carries out. Commands ([below](#the-twist-open-message-and-command-sets)) are
   imperative too (`send-email`, `spawn-agent-run`); the difference is the target
   — a message orders the *model*, a command orders the *runtime* to touch the
   world.

2. **Complete, always.** A message carries **everything its handler needs** to
   compute the next state. The handler performs **no I/O**: it does not read a
   file, query SQLite, call the network, or read the clock. Whatever fact the
   transition depends on — the recipient address, the sender, the exit code, the
   current time, a generated id — is already *in the message*, put there by the
   edge (a subscription or a command effect) that did the I/O before formulating
   the message.

Completeness has a nuance worth stating: "everything the handler needs" means
everything needed *for the state transition*, not every byte of associated
content. A message may carry a *reference* to bulk content (an inbox row id, a
transcript path) when the handler only records the reference and never reads it.
What a message must never do is force the handler to fetch a fact it doesn't
already hold. For example, `route-email` carries the recipient, sender, subject,
and threading keys inline, so routing is a pure decision over the message and the
model — the handler never opens the stored MIME to route.

Because messages are complete and handlers are pure, replay is a total function
of the log: fold the same messages, get the same model, on any machine, with no
I/O and no external state (see *Persistence* below).

### Shape in Go

```go
// A runtime message: a stable tag plus an opaque, serialisable payload.
type Msg struct {
    Tag     string          // imperative, e.g. "route-email"
    Payload json.RawMessage // complete: all the handler needs, no I/O required
    // metadata (id, causation, time) is added by the runtime, not handlers
}

type Handler func(m Model, msg Msg) (Model, []Cmd)

type Cmd struct {
    Tag     string
    Payload json.RawMessage
}

// Effect interprets a command and may emit follow-up messages via `emit`.
type Effect func(ctx context.Context, cmd Cmd, emit func(Msg)) error
```

Payloads are JSON so they serialise into the log cleanly and tolerate schema
drift. A handler decodes its own payload; a decode failure is treated like an
unknown message (dropped, recorded), never a panic.

## Handlers return many commands

`update` returns a *slice* of commands. A single message can fan out: e.g.
`route-email` for a `pay@` address might return `[create-workspace,
lay-in-inputs, provision-workspace, spawn-agent-run]`. The runtime runs them;
each may emit its own follow-up messages, which drive the next step. This keeps
each handler small and the workflow expressed as message → commands → messages,
not as one long imperative function.

## Subscriptions

Some message sources are long-lived and their lifetime depends on state:

- the **Fastmail poller** (or JMAP push stream) that turns new mail into
  `route-email` messages,
- a **ticker** that emits `advance-clock` messages carrying the current time,
- per-running-agent **process watchers** that emit `finish-agent-run` when a
  harness exits.

`subscriptions(model)` returns the set that *should* be running now, each keyed
by a stable ID (e.g. `agent-watch:task-42`). The runtime diffs this against the
currently running sources: it starts sources that newly appear and stops
(cancels the goroutine of) sources that disappear. A source that is still
present across a diff keeps running untouched. This makes "watch the agent while
the task is running, stop watching when it finishes" a pure statement about the
model rather than manual goroutine bookkeeping.

## Concurrency model

- **One reducer goroutine owns the model.** It is the only writer. It reads
  runtime messages from a single inbound channel, calls `update`, applies the
  new model, and hands the returned commands to the interpreter. Because it is
  single-threaded over the model, handlers need no locks and the model needs no
  synchronisation.
- **The command interpreter runs effects concurrently** in a worker pool. An
  effect does I/O (SQLite, network, spawning a harness) and communicates results
  back *only* by emitting messages onto the inbound channel. Effects never touch
  the model directly.
- **Subscriptions run in their own goroutines**, each emitting onto the same
  inbound channel.
- **Everything flows back through the one channel** into the one reducer. The
  model is a sequential fold; all the parallelism lives in the effects.

```
 subscriptions ─┐
 effect workers ─┼──►  inbound channel  ──►  reducer goroutine  ──►  model
 (emit msgs)    ─┘            ▲                     │
                             └──── emit ────────────┘ (commands → interpreter)
```

Backpressure: the inbound channel is buffered; if it fills, emitters block,
which naturally throttles effect workers. Long-running effects (an agent run
that takes minutes) do not block the reducer — they run in a worker and emit a
single `finish-agent-run` message when done, watched by a subscription.

**Edges are message sources.** Subscriptions are not the only things that emit.
The web UI ([08](./08-web-ui.md)) and any other in-process edge inject messages
directly onto the same channel. Out-of-process edges — notably the supervisor
agent's control CLI ([11](./11-supervisor.md)) — reach the runtime through a
loopback **control interface** that answers reads from the model and injects
messages for writes. Every edge, without exception, changes state only by putting
an imperative message on the one channel; there is no other door into the model.

## Persistence: event-sourced state

The model is never saved directly. Instead:

> **Every runtime message that enters the reducer is appended to the message log
> (a SQLite table) before it is applied.** On startup, the log is replayed
> through the same handlers with the command interpreter in **replay mode**,
> where effects are *not* performed. Replaying the messages reconstructs exactly
> the model that existed when the process died.

This works because of the message discipline above (imperative and **complete**,
Principle 1 in [00](./00-vision.md)) plus the determinism rule (Principle 4):

> **Handlers are pure functions of `(model, msg)`. All non-determinism, and all
> I/O results, enter as a complete message.**

Consequences that make replay sound:

- The **output of every effect is itself a message that gets logged.** When the
  Fastmail poller finds mail, it emits `route-email` (logged). When an agent run
  finishes, the watcher emits `finish-agent-run` (logged). During replay we do
  **not** re-run the poller or re-spawn the agent; we simply replay the already
  logged result message. So the log is a complete, self-contained record of
  everything the model ever saw. This is the completeness rule doing its job: the
  message carries the *result* of the I/O, so replay never repeats the I/O.
- **Time comes in on messages.** A handler never calls `time.Now()`. The ticker
  subscription emits `advance-clock{now: ...}`; effects that need a timestamp
  stamp it into the message they emit. Replay uses the recorded timestamps.
- **IDs and randomness come in on messages.** IDs are derived deterministically
  from the message's own log offset, so a handler that needs a new task id
  computes it purely from `(model, msg)` with no fresh randomness. Any value that
  genuinely must come from outside (rare) is generated by an effect and carried
  back inline on the message it emits, so replay reuses the recorded value.

### What is and isn't logged

- **Logged:** runtime messages, in arrival order, with metadata (monotonic id,
  optional causation id, arrival time).
- **Not logged:** the model (it is derived), commands (they are derived by
  handlers), or resolved secrets (a message may carry a `secret://` URL, never
  the plaintext — see [06](./06-secrets.md)).

### Replay mode in detail

There are **no snapshots**. The model is always rebuilt by folding the *entire*
message log from the beginning. The log is the single, complete source of truth
for state; nothing derived from it is persisted. Startup sequence:

1. Open the databases and start from the zero model.
2. Set the interpreter to replay mode: commands are decoded and discarded
   (recorded for debugging) rather than executed. Handlers still run and still
   return commands — we just don't perform them.
3. Fold every logged message, in order, through `update`.
4. Switch the interpreter to live mode.
5. Compute `subscriptions(model)` and start the declared sources. Normal
   operation resumes.

Because effects are suppressed in step 3, replay sends no email, spawns no
agent, writes no outbox row — it only rebuilds RAM state. Any effect that *needs*
to happen after a crash (e.g. an email that was queued but not yet sent) is
handled by reconciliation subscriptions in live mode, driven by the model
(e.g. "for every outbox row still marked `pending`, emit `send-email`"), not by
re-running history.

Full-log replay keeps the model honest — there is no serialisation format to
drift, no snapshot to corrupt, and startup is trivially correct. It is a
deliberate trade for a single-user system where the log stays small enough that
replay is fast; if that ever stops holding, the answer is to prune genuinely dead
history (e.g. drop the message stream of long-`done` tasks), not to cache derived
state.

## Business objects live in RAM

Tasks, routes, the tool/skill registry, and the live state of agent runs are all
fields of the model, held in memory and rebuilt by replay. SQLite is used for
the *content and boundaries* the model refers to — the message log, the
inbox/outbox MIME, archived transcripts and artifacts, and secrets — not to
store the business objects themselves. See [03](./03-persistence.md) for exactly
which data lives where and why.

## Testing implications

Because handlers are pure and effects are data:

- **Handlers** are tested as `(model, msg) -> (model, cmds)` with no mocks.
- **Replay** is tested by folding a fixed message list and asserting the model;
  the same list must always produce the same model.
- **Effects** are tested in isolation against real or faked I/O, asserting the
  messages they emit.
- **Whole flows** are tested by driving messages in and asserting the commands
  that come out at each step (see the walkthroughs in [10](./10-flows.md)).
