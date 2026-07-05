# 09 — Code layout

Two conventions govern the source tree:

1. **Structure by business domain**, not by technical layer.
2. **One thing per file, named after that thing.**

The goal is a tree that reads like a description of the system: opening the
directory listing should tell you what käsi *is* and does.

## Package by domain

Top-level packages are domains, each owning its own messages, commands,
subscriptions, and model slice. There is no `handlers/`, `models/`, or `utils/`
layer cutting across domains — a domain package contains everything about that
domain.

```
kasi/
├── cmd/kasi/            # main(): wire runtime + domains, open DBs, start
├── runtime/             # the Elm-style core (domain-agnostic)
│   ├── model.go              # Model aggregate; composed of domain slices
│   ├── message.go            # Msg type, tags, registry
│   ├── command.go            # Cmd type, interpreter, replay vs live mode
│   ├── subscription.go       # Sub type, diffing, lifecycle
│   ├── loop.go               # the reducer goroutine + inbound channel
│   └── log.go                # append + full-log replay against message_log
├── email/               # Fastmail JMAP, inbox/outbox, routing ([04])
├── agents/              # harness invocation, agent runs, transcripts ([05])
├── tasks/               # task lifecycle, workspaces ([05])
├── requests/            # agent-raised UI requests: model, messages, form spec ([08])
├── mime/                # MIME parse/build, part<->file mapping ([02])
├── skills/              # skill registry + provisioning ([07])
├── tools/               # mise integration, tool registry ([07])
├── secrets/             # secrets DB, secret:// resolver ([06])
├── web/                 # dispatch routes, htmlc components, Turbo ([08])
└── store/               # SQLite access shared by domains ([03])
```

A domain package (e.g. `email/`) declares its own runtime messages, the commands
that perform its effects, and the subscriptions that feed it, and registers them
with the runtime. Adding a capability is adding/extending a domain package and
registering its tags — no central union to edit ([01](./01-architecture.md)).

## One thing per file, named after the thing

Within a package, each concrete thing gets its own file, prefixed by its kind.
The prefix makes the file's role obvious and groups related files in a listing.

Message and command files are named after their **imperative tag**
([01](./01-architecture.md)), so the filename *is* the tag in snake_case.

| Prefix | Contains | Example |
|--------|----------|---------|
| `message_*.go` | one runtime message type (imperative) + its handler(s) | `message_route_email.go` |
| `command_*.go` | one command type + its effect (interpreter) | `command_send_email.go` |
| `subscription_*.go` | one subscription source | `subscription_inbox_poll.go` |
| `model_*.go` | one business object / model slice | `model_task.go` |
| `view_*.go` / `*.vue` | one UI view/component ([08]) | `view_task.vue` |

Illustrative listing for the `email/` package:

```
email/
├── model_route.go                 # Route table + initiator allowlist (model slice)
├── message_route_email.go         # "route-email" + handler (auth, route, thread)
├── message_add_collaborator.go    # "add-collaborator" + handler (CC -> participant)
├── message_allow_sender.go        # "allow-sender" / "revoke-sender" + handlers
├── message_mark_reply_queued.go   # "mark-reply-queued" + handler
├── message_mark_email_sent.go     # "mark-email-sent" + handler (mark sent)
├── command_assemble_reply.go      # build MIME reply from out/  ([02])
├── command_send_email.go          # JMAP EmailSubmission effect
├── subscription_inbox_poll.go     # JMAP Email/changes poller
├── routing.go                     # local-part -> template selection
└── jmap.go                        # thin JMAP client (stdlib http+json)
```

And `tasks/`:

```
tasks/
├── model_task.go                  # Task struct + state machine + participants
├── message_finish_agent_run.go    # "finish-agent-run" + handler (harvest/reply/skill)
├── message_finish_task.go         # "finish-task" + handler (archive+cleanup)
├── command_create_workspace.go    # make $WORKDIR/task-$ID, in/, out/
├── command_lay_in_inputs.go       # write MIME parts into in/  ([02])
├── command_harvest_output.go      # read out/ into MIME parts ([02])
├── command_archive_task.go        # archive-then-delete ([05])
└── workspace.go                   # workspace path helpers
```

And `requests/` (the agent-raised UI request, [08](./08-web-ui.md)):

```
requests/
├── model_ui_request.go            # UIRequest struct + form spec + status
├── command_mint_ui_request.go     # "mint-ui-request": token, row, link  ([03])
├── message_register_ui_request.go # "register-ui-request" + handler (drives reply)
├── message_answer_ui_request.go   # "answer-ui-request" + handler (lay-in + resume)
└── command_lay_in_answers.go      # write answers/uploads into in/  ([05])
```

The form itself is rendered and posted in `web/` (e.g. `view_request.vue` plus the
token-validated GET/POST routes), which turns a submission into the
`answer-ui-request` message ([08](./08-web-ui.md)) — request *state and rules*
live in `requests/`, request *rendering* lives in `web/`.

The pattern generalises: a reader can predict the filename for "the thing that
sends email" (`command_send_email.go`) or "the message that finishes an agent
run" (`message_finish_agent_run.go`) without searching — and because message tags
are imperative, the filename reads as the instruction it carries.

## Rules of thumb

- **A message and its handler live together.** The `message_*.go` file both
  defines the tag/payload and registers the handler. You never hunt across the
  tree to find who handles a tag.
- **A command and its effect live together.** `command_*.go` defines the command
  and its interpreter, including any `secret://` resolution it needs
  ([06](./06-secrets.md)).
- **Domain slices compose into the model.** `runtime/model.go` aggregates each
  domain's `model_*` slice; domains don't reach into each other's state, they
  emit messages.
- **`runtime/` is domain-agnostic.** It knows about `Msg`, `Cmd`, `Sub`, the log,
  and the loop — never about tasks or email. Domains depend on `runtime`, not the
  reverse.
- **Cross-domain interaction is by message, not by call.** `email/` doesn't call
  into `tasks/`; it emits `route-email` and `tasks/` handles it. This keeps the
  open-set, replayable design intact ([01](./01-architecture.md)).
- **Keep files small and single-purpose.** If a file needs an "and" to describe
  it, it is probably two files.
