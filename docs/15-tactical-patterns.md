# 15 — Tactical patterns

This is the pattern book for writing käsi's Go: the canonical shape of a
message file, a command file, a model slice, a module, a subscription. Open it
the first time you write any of these; follow it every time after. The shapes
exist so that every file of a given kind reads the same way — and so that the
architectural rules ([01](./01-architecture.md)) are enforced by construction,
not by review vigilance.

The code below is canonical in *shape*, illustrative in detail — exact helper
names may differ in the source; the structure, signatures, and rules do not.

## The one-liners

Every rule in this document, compressed:

1. **A tag literal appears exactly once in the codebase** — in the file that
   owns it. Everyone else uses the constant or the constructor.
2. **Every message and command is: tag constant + payload struct +
   constructor.** Nobody hand-builds a `Msg` or `Cmd` from strings and JSON.
3. **Handlers receive typed payloads and their own slice; they return their
   own slice.** Write-ownership is in the function signature, not in a
   convention.
4. **Seam contracts live in leaf `msg/` packages** so cross-domain
   construction is type-checked and import cycles are impossible.
5. **Effects see edges and payload, never the model.** Results leave an
   effect only as emitted messages, built with constructors.
6. **Model slices are plain values.** No I/O, no locks, no pointers handed
   out, ids derived from the log.
7. **`module.go` is a table of contents**; `main.go` is the only place
   modules meet edges ([01](./01-architecture.md)).

## Messages: `message_*.go`

One file per tag, containing the tag constant, the payload struct, the
handler, and its registration — nothing else ([09](./09-code-layout.md)).

```go
// tasks/message_create_task.go
//
// "create-task" — sent by email's route-email handler ([04], [10]).
// Owns: creating the Task, seeding participants, kicking off the workspace.

func registerCreateTask(mod *runtime.Module) {
    runtime.HandleMsg(mod, msg.CreateTask, handleCreateTask)
}

func handleCreateTask(v runtime.View, s Model, p msg.CreateTaskPayload,
    meta runtime.Meta) (Model, []runtime.Cmd) {

    id := TaskID(meta.Offset) // deterministic: derived from the log position ([01])

    s.Tasks = s.Tasks.With(id, Task{
        ID:           id,
        Status:       Open,
        Route:        p.Route,
        Template:     p.Template,
        ThreadKey:    p.MessageID,
        Participants: participants(p.Sender, p.Cc),
    })

    return s, []runtime.Cmd{
        NewCreateWorkspace(id),
        NewLayInInputs(id, p.InboxID),
        NewProvisionWorkspace(id, p.Template),
        runtime.Send(agentmsg.NewSpawnAgentRun(id)),
    }
}
```

What the generic helper does — and why handlers look like this:

- `runtime.HandleMsg[S, P]` wraps the raw handler shape of
  [01](./01-architecture.md) (`func(Model, Msg) (Model, []Cmd)`). It decodes
  the payload into `P` — **a decode failure drops the message, recorded,
  never a panic** — pulls the domain's slice `S` out of the model, and puts
  the returned slice back. Registration also hands the runtime the payload
  *prototype*, which is what lets the test runner strict-decode script
  `send`s against the real struct ([14](./14-test-language.md)).
- The signature is the ownership rule made physical: the handler can **read**
  everything (`v runtime.View`) but can only **return its own slice**. There
  is no way to write another domain's state, because there is nowhere to put
  it.
- Everything the handler needs is in `p` and `meta` — the completeness rule
  ([01](./01-architecture.md)). `meta` carries what the runtime stamps (log
  offset, causation, arrival time); note `TaskID(meta.Offset)`: identity
  without randomness.

A handler body may never contain: `time.Now()`, `rand`/UUID generation, file
or network or SQL access, goroutines, channels, locks, or a retained pointer
into anything mutable. If a handler seems to need one of these, the fact it
wants is missing from the message — produce it at the edge and carry it in
([01](./01-architecture.md)).

## Seams: the `msg/` leaf packages

`email` constructs `create-task` for `tasks`; `tasks` returns `assemble-reply`
to be interpreted by `email`. If those constructors lived in the domain
packages, the two would import each other — a cycle Go rejects. So:

> **A domain's seam — the messages others may send it, and the commands
> others may return for it — lives in `<domain>/msg`, a leaf package that
> imports nothing but `runtime/`.**

```go
// tasks/msg/create_task.go
package msg // imported as taskmsg

const CreateTask = "create-task"

type CreateTaskPayload struct {
    InboxID   int64    `json:"inbox_id"`
    Route     string   `json:"route"`
    Template  string   `json:"template"`
    Sender    string   `json:"sender"`
    Cc        []string `json:"cc"`
    Subject   string   `json:"subject"`
    MessageID string   `json:"message_id"`
}

func NewCreateTask(p CreateTaskPayload) runtime.Msg {
    return runtime.NewMsg(CreateTask, p)
}
```

Any domain may import any other's `msg/` package; cycles are structurally
impossible because `msg/` packages import only `runtime/`. Tags and payloads
that are *internal* to a domain (nobody else sends them) stay in the domain
package itself.

The seam packages are also the unit of agreement for parallel work
([12](./12-development-process.md)): committing `tasks/msg` *is* agreeing the
contract, and `email/` can be built and tested against it — `use email` plus
the `dropped` read ([14](./14-test-language.md)) — before `tasks/` exists.

## Crossing domains: reads and writes

Writes are `send`, always ([01](./01-architecture.md)). The email side of the
hand-off worked in [10](./10-flows.md):

```go
// email/message_route_email.go
//
// "route-email" — announced by the inbox subscription for every stored mail.
// Email's competence: authorise the sender, resolve the route. The task
// itself is tasks' business, reached by send ([01]).

func handleRouteEmail(v runtime.View, s Model, p RouteEmailPayload,
    meta runtime.Meta) (Model, []runtime.Cmd) {

    if id, ok := tasks.ByThreadKey(v, p.InReplyTo); ok {
        // Reply within an existing task. Whether the sender is a participant
        // is tasks' state, so tasks' handler checks it — not us.
        return s, []runtime.Cmd{runtime.Send(taskmsg.NewAppendToTask(
            taskmsg.AppendToTaskPayload{
                TaskID: id, InboxID: p.InboxID, Sender: p.Sender, Cc: p.Cc,
            }))}
    }

    if !s.Initiators.Allows(p.Sender) {
        return s, nil // not an error: the stored mail simply stays ignored ([04])
    }

    route := s.Routes.For(localPart(p.Recipient))
    return s, []runtime.Cmd{runtime.Send(taskmsg.NewCreateTask(
        taskmsg.CreateTaskPayload{
            InboxID: p.InboxID, Route: route.Name, Template: route.Template,
            Sender: p.Sender, Cc: p.Cc, Subject: p.Subject, MessageID: p.MessageID,
        }))}
}
```

Reads cross domains through **exported pure functions over the View**, owned
by the domain whose state they read — `tasks.ByThreadKey(v, key)` above. Two
rules keep this tidy:

- A read helper takes the `View` and returns plain values (ids, copies) —
  never a pointer into the slice.
- Read imports must be **one-directional** between any two domains (`email`
  reads `tasks`; `tasks` never reads `email`). If both directions seem
  necessary, one of the two decisions is in the wrong domain — move the
  decision to the owner of the state, as with the participant check above.

## Commands: `command_*.go`

One file per tag: constant, payload, constructor, effect, registration. The
constructor is how *handlers* stay typo-proof; the effect is where the world
appears.

```go
// email/command_send_email.go
//
// "send-email" — transmit one pending outbox row via the mail edge ([04]).
// Idempotent: the pre-generated Message-ID makes a resend detectable ([03]).

const SendEmail = "send-email"

type SendEmailPayload struct {
    OutboxID  int64  `json:"outbox_id"`
    MessageID string `json:"message_id"`
}

func NewSendEmail(outboxID int64, messageID string) runtime.Cmd {
    return runtime.NewCmd(SendEmail, SendEmailPayload{
        OutboxID: outboxID, MessageID: messageID,
    })
}

func registerSendEmail(mod *runtime.Module) {
    runtime.HandleCmd(mod, SendEmail, sendEmailEffect)
}

func sendEmailEffect(ctx context.Context, e Edges, p SendEmailPayload,
    emit runtime.Emit) error {

    raw, err := e.Store.OutboxRaw(ctx, p.OutboxID)
    if err != nil {
        return err // recorded; the reconciliation subscription retries ([03])
    }
    if err := e.Mail.Submit(ctx, raw); err != nil {
        return err
    }
    emit(NewMarkEmailSent(p.OutboxID, e.Clock.Now()))
    return nil
}
```

The effect's discipline mirrors the handler's, inverted:

- It sees **edges and payload — never the model, never the View**. If an
  effect needs a fact from the model, the handler that returned the command
  should have put that fact in the payload.
- Its results leave **only as emitted messages**, built with constructors,
  complete — note the timestamp comes from `e.Clock`, an edge, so the sim
  clock controls it ([13](./13-testing.md)).
- A returned error is recorded, and recovery is the *reconciliation*
  pattern — model-driven retry by a subscription ([03](./03-persistence.md))
  — never a hidden retry loop inside the effect.
- Any `secret://` resolution happens here, at the edge, last
  ([06](./06-secrets.md)).

## Model slices: `model_*.go`

```go
// tasks/model_task.go

type TaskID int64

type Status string

const (
    Open          Status = "open"
    AwaitingAgent Status = "awaiting-agent"
    AwaitingUser  Status = "awaiting-user"
    Done          Status = "done"
)

type Task struct {
    ID           TaskID
    Status       Status
    Route        string
    Template     string
    ThreadKey    string
    Participants []string
    Runs         []AgentRunID
}

// Model is the tasks slice of the application model.
type Model struct {
    Tasks kv.Map[TaskID, Task] // copy-on-write map: With/Without return a new Map
}

// Pure read helpers — the only way other domains see tasks' state.

func ByThreadKey(v runtime.View, key string) (TaskID, bool) { /* … */ }

func (t Task) IsParticipant(addr string) bool { /* … */ }
```

- **Plain values.** Typed ids (`TaskID`, not `int64`), string-typed enums
  that match the vocabulary the docs and test scripts use (`awaiting-user` —
  the scripts assert on these exact strings, [14](./14-test-language.md)).
- **Copy-on-write containers, not raw maps.** `s.Tasks.With(id, t)` returns a
  new map value. This is what makes "hand a read snapshot to another
  goroutine" safe: the reducer is the single writer
  ([01](./01-architecture.md)), and readers hold immutable values, so there
  are no locks anywhere in a domain.
- **No I/O, no JSON, no time.** A model file imports other model files and
  little else. If a `model_*.go` file grows an `import "database/sql"`, it
  has stopped being a model file.

## Modules: `module.go` and `main.go`

`module.go` is a table of contents; reading it tells you every tag the domain
owns:

```go
// tasks/module.go

// Edges is everything tasks touches in the world. Real implementations are
// wired in main.go; simulated twins live in this package ([12]).
type Edges struct {
    Store     Store            // outbox/archive rows ([03])
    Workspace Workspace        // $WORKDIR trees ([05])
    Clock     runtime.Clock
}

func Module(e Edges) runtime.Module {
    mod := runtime.NewModule("tasks", Model{}, e)

    registerCreateTask(mod)
    registerAppendToTask(mod)
    registerFinishAgentRun(mod)
    registerFinishTask(mod)

    registerCreateWorkspace(mod)
    registerLayInInputs(mod)
    registerHarvestOutput(mod)
    registerArchiveTask(mod)

    return mod
}

// SimEdges is the full simulated set — what `kasi test` assembles by
// default, and the twin the seam rule demands ([12]).
func SimEdges() Edges { /* … */ }
```

And `main.go` is the one assembly ([01](./01-architecture.md)) — modules meet
real edges here and nowhere else:

```go
// cmd/kasi/main.go (the serve path)

app := runtime.New(
    email.Module(email.Edges{Mail: jmapClient, Store: store, Clock: clock}),
    tasks.Module(tasks.Edges{Store: store, Workspace: workdir, Clock: clock}),
    agents.Module(agents.Edges{Harness: claude.New(cfg), Clock: clock}),
    skills.Module(skills.Edges{Store: store}),
    // …every module, in the open; absent here = absent from the program
)
app.Run(ctx)
```

No `init()`, no globals, no registration side effects on import: constructing
two apps in one test process yields two disjoint worlds
([13](./13-testing.md)).

## Subscriptions: `subscription_*.go`

A subscription file exports one thing: a pure function from state to the set
of sources that should be running, each with a stable id
([01](./01-architecture.md)). The runtime diffs; the source's body is an
edge-style function (edges + emit, no model):

```go
// agents/subscription_agent_watch.go
//
// One watcher per running agent run; emits "finish-agent-run" when the
// harness process exits ([05]).

func agentWatchSubs(v runtime.View, s Model) []runtime.Sub {
    var subs []runtime.Sub
    for _, r := range s.RunningRuns() {
        r := r
        subs = append(subs, runtime.Sub{
            ID: fmt.Sprintf("agent-watch:%d", r.Task),
            Run: func(ctx context.Context, e Edges, emit runtime.Emit) {
                res := e.Harness.Wait(ctx, r.Handle)
                emit(NewFinishAgentRun(r.Task, r.ID, res.Exit,
                    res.TranscriptPath, res.OutManifest, res.Stopped))
            },
        })
    }
    return subs
}
```

Lifecycle is entirely the runtime's: a run appearing in the model starts a
watcher, a run leaving it cancels one. The body never loops-and-sleeps to
poll the model — if it wants model state, it is a handler wearing the wrong
hat.

## Where each fact may live — a checklist

When writing a new capability, place each ingredient by this table; if
something has no row, it probably belongs to an edge:

| Ingredient | Lives in | Never in |
|-----------|----------|----------|
| A tag string | Its own file (constant), once | Call sites, tests, other domains |
| A payload's field names | The payload struct (json tags) | Duplicated structs elsewhere |
| The current time | A message/payload field, stamped by an edge's `Clock` | `time.Now()` in a handler or model |
| A new id | `meta.Offset` (or an edge, carried back on a message) | `rand`/UUID in a handler |
| A decision over state | A handler in the state's owning domain | An effect, an edge, another domain |
| I/O of any kind | An effect or subscription body, through an edge | Handlers, model files, `msg/` packages |
| A cross-domain instruction | `runtime.Send` of the other domain's `msg/` constructor | A direct call, a shared slice write |
| A secret | `secret://` URL until inside an effect ([06](./06-secrets.md)) | Payloads, the model, logs |
