# 11 — The supervisor

käsi's orchestration — routing mail, spawning runs, moving results — is done by
the runtime, in code ([01](./01-architecture.md), [05](./05-agents-and-tasks.md)).
But you can also **talk to käsi about itself**. The **supervisor** is a
conversational agent with tools over käsi's own data model: email it and ask it to
list, inspect, stop, resume, or archive tasks and UI requests in plain language.

Think of it as the main agent's conversational face. As code it orchestrates
silently; when you address it, it runs an agent session whose tools operate the
system.

## Reaching the supervisor

The supervisor is a **reserved route** ([04](./04-email.md)) — e.g.
`kasi@kasi.decode.ee` — bound to a **supervisor task template**
([07](./07-skills-and-tools.md)) whose agent is provisioned with käsi's control
CLI and a skill describing it. Emailing it starts an ordinary task — one thread,
one session, transcript captured, resumable ([05](./05-agents-and-tasks.md)) —
that just happens to have the whole system as its subject.

Only **initiator-allowlisted** senders can reach it ([04](./04-email.md)); the
supervisor's tools are powerful, so this address is owner-only.

## What it can do

Read and act across the business objects ([02](./02-object-model.md)):

| Ask | Kind | Effect |
|-----|------|--------|
| "what's running right now?" | read | list tasks by status |
| "show me task 42's latest transcript" | read | inspect a task and its agent runs |
| "stop task 42, it's off track" | act | `stop-agent-run` ([05](./05-agents-and-tasks.md)) |
| "resume 42 with this correction" | act | `spawn-agent-run` (resume session) |
| "archive 42, it's fine" | act | `finish-task` (archive + cleanup) |
| "what UI requests are open?" | read | list `ui_request`s ([02](./02-object-model.md)) |
| "cancel the request on task 17" | act | `expire-ui-request` |
| "which skills do we have?" | read | list the skill registry ([07](./07-skills-and-tools.md)) |

The same operations are available in the web UI ([08](./08-web-ui.md)); the
supervisor is the natural-language, email-native way to do them when you'd rather
ask than click.

## How it stays inside the design

The supervisor never mutates the model directly — that would break replay and the
single-writer invariant ([01](./01-architecture.md)). Instead its tools obey the
same rule as every other edge:

- **Reads** are served from the in-RAM model.
- **Writes are imperative messages** ([01](./01-architecture.md)). "Stop task 42"
  becomes a `stop-agent-run` message; "archive it" becomes `finish-task`. Each
  goes through the log and the one reducer, exactly as if it came from the web UI
  or an email. So every supervisor action is **audited in the message log** and
  **replayable** like anything else.

The supervisor is, in the end, just another message source ([08](./08-web-ui.md)) —
one whose "form" is a conversation and whose "submit" is a tool call.

## The control interface

In-process edges (web, email ingestion, subscriptions) inject messages and read
the model **directly**, because they run inside the käsi process. The supervisor's
agent runs **out-of-process** — it is a harness in a workspace
([05](./05-agents-and-tasks.md)) — so it cannot reach into the model. It goes
through the **control interface**:

- The runtime exposes a **loopback-only control endpoint** (a Unix socket on the
  exe.dev box). It answers read queries from the model and accepts message
  injections.
- The supervisor's workspace is provisioned with the **`kasi` binary itself**
  (as a tool, via mise — [07](./07-skills-and-tools.md)) — there is only one
  executable, and its control subcommands are thin clients to that socket.
  `kasi tasks list` is a read; `kasi task stop 42` injects a `stop-agent-run`
  message.
- Because the endpoint is loopback-only and the socket lives on a private
  exe.dev host ([08](./08-web-ui.md)), it needs no auth of its own — the same
  host-gated trust model as the web UI.

```
supervisor agent ── kasi CLI ──► control interface (loopback socket)
                                        │
                          reads ◄───────┤ (from the in-RAM model)
                          writes ───────┴──► inject imperative message ──► reducer
```

This is what "the main agent operates on the app's data model" means concretely:
a conversational agent, equipped with a control CLI, reading the model and
emitting the very same messages the rest of the system emits — no privileged
back door, everything on the log.

## Guards

- **Owner-only.** Reserved route, initiator-allowlist gated ([04](./04-email.md)).
- **Auditable.** Every action is a logged message; the supervisor's own transcript
  ([05](./05-agents-and-tasks.md)) plus the log together record what it did and
  why.
- **No self-foot-guns.** The supervisor should not stop or archive *its own* task
  mid-thought; the control CLI refuses operations on the caller's own task id so a
  supervisor run can't terminate itself before finishing its reply.
