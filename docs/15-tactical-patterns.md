# 15 — Tactical patterns

This is the pattern book for writing käsi's Go: the canonical shape of a
message file, a command file, a model slice, a module, a subscription. Open it
the first time you write any of these; follow it every time after. The shapes
exist so that every file of a given kind reads the same way — and so that the
architectural rules ([01](./01-architecture.md)) are enforced by construction,
not by review vigilance.

The code below is canonical in *shape*, illustrative in detail — exact helper
names may differ in the source; the structure, signatures, and rules do not.

These shapes are also executable: the `kasi` [kit](https://github.com/dhamidi/kit)
provider (`providers/kasi/`, tools pinned in `mise.toml`) lists, inspects, and
generates them — `kit component list` shows every module, message, command,
model object, and subscription in the codebase (discovered structurally with
ast-grep, not by convention-guessing), and `kit generate` / `kit manifest
apply` bootstrap new ones in exactly the forms below. If this document and
the provider's templates ever disagree, one of them is wrong — fix both in
the same change.

## The one-liners

Every rule in this document, compressed:

1. **A tag literal appears exactly once in the codebase** — in the file that
   owns it. Everyone else uses the constant or the constructor.
2. **Every message and command is: tag constant + payload struct +
   constructor.** Nobody hand-builds a `Msg` or `Cmd` from strings and JSON.
3. **Handlers receive typed payloads and their own slice; they return their
   own slice.** Write-ownership is in the function signature, not in a
   convention.
4. **Contracts live in leaf `msg/` packages** so cross-domain
   construction is type-checked and import cycles are impossible.
5. **Effects see edges and payload, never the model.** Results leave an
   effect only as emitted messages, built with constructors.
6. **Model slices are plain values.** No I/O, no locks, no pointers handed
   out, ids derived from the log.
7. **`module.go` is a table of contents**; `main.go` is the only place
   modules meet edges ([01](./01-architecture.md)).
8. **Views render View structs.** htmlc receives `map[string]any`, and every
   value in it is a named `<Name>View` struct built by the route handler —
   never a raw model object, never an ad-hoc map.
9. **Forms are objects that become one message.** A form object binds the
   request (binding never fails), validates itself, re-renders the same view
   carrying its values and errors when invalid — and constructs exactly one
   imperative runtime message when valid.

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

## Contracts: the `msg/` leaf packages

`email` constructs `create-task` for `tasks`; `tasks` returns `assemble-reply`
to be interpreted by `email`. If those constructors lived in the domain
packages, the two would import each other — a cycle Go rejects. So:

> **A domain's contract — the messages others may send it, and the commands
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

The contract packages are also the unit of agreement for parallel work
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

func Module(e Edges) *runtime.Module {
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
// default, and the simulated twin the twin rule demands ([12]).
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

## Views: `view_*.go` + `view_*.vue`

The web UI renders htmlc components ([08](./08-web-ui.md)); each view is a
pair of files in `web/` ([09](./09-code-layout.md)) — the `.vue` template and
its Go side:

```go
// web/view_task.go

// TaskView is the data view_task.vue renders — one task: thread,
// participants, runs, artifacts.
type TaskView struct {
	ID           string
	Status       string
	Subject      string
	Participants []string
	Runs         []AgentRunView
}

// RenderTask writes the full page ([08]).
func RenderTask(w io.Writer, engine *htmlc.Engine, view TaskView) error {
	return engine.RenderPage(w, "view_task", map[string]any{
		"task": view,
	})
}
```

- **One View struct per view, named `<Name>View`.** htmlc receives a
  `map[string]any`, and idiomatically **every value in that map is a
  struct** like this one — built from the model by the route handler, never
  a raw model object and never an ad-hoc map. Handing a template a model
  object couples the template to the model's shape (and hands it data the
  page has no business seeing); building the map field-by-field scatters
  the page's data shape across the handler. The View struct is the explicit,
  greppable answer to "what does this page show," and the model→view mapping
  in the handler is where presentation concerns (formatting dates, sizes,
  truncation) live — keeping both the model and the template clean.
- **Pages use `RenderPage`; Turbo-swapped fragments use `RenderFragment`**
  ([08](./08-web-ui.md)). Nothing else about the pair differs.
- **`web/` may read domains; domains never import `web/`.** Mapping model to
  View structs is a read like any other edge read.
- **A view never writes.** Forms post to dispatch routes whose handlers emit
  imperative runtime messages ([08](./08-web-ui.md)) — the same front door
  as everything else.

## Forms: `form_*.go`

Views read; **form objects** write. Every UI write follows one loop
([08](./08-web-ui.md)):

```
browser ──form──► handler: bind + validate
   │  invalid: re-render the same view — the form, carrying its
   │           values and errors, is one of the props-map structs
   ▼  valid
construct one imperative message ──► reducer ──► model updated
   ▼
redirect → GET → View structs from the new model ──► htmlc ──► browser
```

```go
// web/form_allow_sender.go

// AllowSenderForm — add an address to the initiator allowlist ([04]).
type AllowSenderForm struct {
	Address string
	Errors  FormErrors
}

// Binding never fails — bad input becomes field errors, not an HTTP error.
func BindAllowSenderForm(r *http.Request) AllowSenderForm {
	return AllowSenderForm{
		Address: strings.TrimSpace(r.FormValue("address")),
		Errors:  FormErrors{},
	}
}

func (f AllowSenderForm) Validate() AllowSenderForm {
	if f.Address == "" {
		f.Errors.Set("address", "an email address is required")
	}
	return f
}

func (f AllowSenderForm) Valid() bool { return len(f.Errors) == 0 }

// Message constructs the one imperative message a valid submission means.
func (f AllowSenderForm) Message() runtime.Msg {
	return msg.NewAllowSender(msg.AllowSenderPayload{Address: f.Address})
}
```

And the handler, which is now almost policy-free:

```go
form := BindAllowSenderForm(r).Validate()
if !form.Valid() {
	w.WriteHeader(http.StatusUnprocessableEntity)
	return RenderSettings(w, engine, SettingsView{AllowSender: form /* … */})
}
app.Send(form.Message())
http.Redirect(w, r, routes.Path("settings"), http.StatusSeeOther)
```

The rules:

- **Form fields are raw strings; rich values parse through `flag.Value`.**
  A form's own fields hold exactly what the browser sent, so a re-render
  always echoes what was typed — even when it didn't parse. A field that
  *means* more than a string is a **named domain type implementing the
  stdlib `flag` package's `Value` interface**: `Set(string) error` parses
  and validates (its error text *is* the field's error message), `String()`
  renders the value back. `FormErrors.Parse(field, raw, &v)` binds one.
  The payoff for playing nice with the stdlib contract: the same types
  parse real CLI flags in the `kasi` control subcommands via `flag.Var`
  ([11](./11-supervisor.md)), and "what is a valid X" lives in exactly one
  place — the type — not scattered across handlers.

  ```go
  // email/model_route.go — the owning domain defines the rich value.

  // MaxTurns — how many agent turns a route allows per task.
  type MaxTurns int

  func (t *MaxTurns) Set(raw string) error {
  	n, err := strconv.Atoi(raw)
  	if err != nil || n < 1 {
  		return fmt.Errorf("must be a whole number of turns, at least 1")
  	}
  	*t = MaxTurns(n)
  	return nil
  }

  func (t MaxTurns) String() string { return strconv.Itoa(int(t)) }
  ```

  ```go
  // web/form_update_route.go — the form parses it in Validate.
  func (f UpdateRouteForm) Validate() UpdateRouteForm {
  	var turns email.MaxTurns
  	f.Errors.Parse("max_turns", f.MaxTurns, &turns)
  	return f
  }
  ```

- **A form object is a View struct with a memory of what went wrong.** It
  goes into the props map like any other struct, so re-rendering with errors
  is the *same* render path as the first render — no separate error page,
  no flash-message machinery. `FormErrors` is a flat `field → message` map
  templates read directly (`v-if="form.Errors.address"`).
- **Bind, validate, and message-construction are three separate steps** on
  one value, so the scenario suite can drive a form through the web edge and
  assert each: bad input re-renders with the right error; good input emits
  exactly the right message ([14](./14-test-language.md)).
- **One valid submission, one message.** The form is the last stop before
  the front door; whatever the submission *means* is said as a single
  imperative message the owning domain handles. A form needing to emit two
  messages is two forms — or the domain is missing a message.
- **Messages the web emits are contract messages.** The web edge is another
  domain boundary: a tag it constructs belongs in the owning domain's
  `msg/` package, like any cross-domain send.
- **POST/redirect/GET closes the loop.** The web edge's `Send` blocks until
  the reducer has applied the message, so the redirected `GET` renders the
  new model — the browser never sees a stale page after a successful write.

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
| Data a template renders | A `<Name>View` struct in `web/`, one per view | Raw model objects or ad-hoc maps in the props map |
| A UI write | A `<Name>Form` object: bind → validate → one message | Handlers parsing/validating inline, or emitting several messages |
| A string that means more | A named type implementing `flag.Value` (`Set` validates, `String` renders) | `strconv` calls scattered across forms and handlers |
