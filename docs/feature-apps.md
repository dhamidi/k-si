# Apps

*the agent builds it, käsi keeps it running*

Sometimes the right answer to a request isn't an email — it's a little web app. A
dashboard for the numbers you keep asking about, a form that kicks off a task, a
viewer for something the agent scraped. The agent can already write that code. What
it couldn't do is *keep it running* and put it somewhere you can open.

Apps close that gap. The agent builds a small web app, hands it to käsi with one
command, and käsi keeps it alive — on its own port, behind the same private front
door as everything else on the machine. A one-off ("build me a page that shows my
Wise balance") becomes a URL you can bookmark.

An app isn't only something you open, though. The operations behind it are also an
API the agent can call while it works — so an app is a durable tool the agent *uses*,
not just a page you visit. That's what turns "forward me the receipt" into "file it in
the accounting app," and it's covered under *The agent uses apps*, below.

Status: this is the design of record. The store and the control-command plumbing it
builds on exist (Flow F, decision-014); the app runner and its systemd wiring are
not yet built.

## Where apps live

An app is a directory in the store: `./store/apps/<name>/`. Code and the app's own
data both live there, so — like everything in the store — an app persists across
tasks and sits outside the event log. The agent writes it directly.

The agent is told this in its standing instructions, next to what it already knows
about `./store/`: build web apps under `./store/apps/<name>/`, and register them
with `kasi app`.

An app is just two things: **a directory, and a command that starts it.** The
command must do one thing käsi's way — read `$PORT` from its environment and serve
on it. käsi assigns the port; the app listens there and owns its whole root, exactly
like a normal server. There's no base path to thread through and no prefix to strip,
because the app has the port to itself.

## Building one

The store directory is the playground. The agent scaffolds an app under
`./store/apps/<name>/`, writes it in whatever it likes — bun, a Go binary, a Python
script — installs its dependencies, and runs it to try it out. Nothing is permanent
until it's registered, so the agent can iterate freely.

Because the app honors `$PORT`, "run it to try it" and "let käsi run it for real" are
the same program — registering doesn't change how the app starts, only who starts it
and whether käsi is keeping it up.

An app can offer more than a page. The operations behind its UI — the endpoints that
add a row, run an export, flip a setting — double as an API the agent can call on
`localhost` while it works a task. Build those if you want the app to be something the
agent *uses*, and describe them in an `app.json` in the app directory (each
operation's method, path, purpose, and payload). A pure-UI app just leaves it out.

## Registering

When the app works, the agent registers it:

```bash
kasi app add balance --start "bun run server.ts"
```

`kasi app` is a control command, a sibling of `kasi notify`: it reads the `KASI_*`
variables already in the run's environment and POSTs to käsi's host-gated control
endpoint, which does the real work. On `add`, käsi:

- checks the name is a slug (it becomes a service name),
- assigns a free port in exe.dev's forwarded range (3000–9999),
- reads the app's operations from its `app.json`, if it has one,
- writes a systemd user unit and starts it, and
- records the registration — name, port, start command, operations — in the event log.

Then it returns the app's URL — `https://<vm>.exe.xyz:<port>/` — so the agent can
open it and confirm it's live. To change the start command or pick up new code, run
`add` again; the registration is replaced in place, keyed by name, the way
remembering an existing memory updates it.

## How it's exposed

käsi doesn't proxy anything, and it never touches an app's traffic. The app listens
on its port, and exe.dev's proxy forwards every port in the 3000–9999 range straight
through — handling the certificate and TLS — so the app is reachable at
`https://<vm>.exe.xyz:<port>/` with nothing for käsi to route.

Protection is automatic and identical to käsi's own UI. Those forwarded ports are
private: only people with access to the VM can reach them, and a first request bounces
through the exe.dev login. That's what "behind the same auth proxy" means — the app
inherits the host's IAM without käsi or the app doing a thing. And because the app
sits behind that login, exe.dev hands it the caller's identity in `X-ExeDev-Email`
and `X-ExeDev-UserID` headers, so an app that wants per-user rules can enforce them.

Making an app truly public — unauthenticated — is a manual exe.dev step
(`share set-public`), and only one port per VM can be public. käsi keeps apps private
by default and stays out of that decision.

## Running it

käsi doesn't babysit the process — systemd does. Registering writes a
`systemctl --user` unit with lingering on, so the app starts on boot, restarts if it
crashes, and logs to journald. käsi generates and manages the unit; the process's
life is systemd's job, not the reducer's.

That split is what makes restarts clean. If käsi restarts, it rebuilds its registry —
which name holds which port, which units should exist — from the log, while systemd
kept every app running in the meantime. The registry is käsi's state; the running
processes are the machine's.

The rest of the CLI manages the fleet:

```bash
kasi app ls               # every registered app, its URL, and whether it's up
kasi app logs balance     # tail journald for one app
kasi app restart balance  # bounce it after editing the code
kasi app rm balance       # stop it, drop the unit, forget the registration
```

`rm` stops the app and drops it from the log; it leaves the code in
`./store/apps/balance/`, so you can add it back later. Deleting the code is a store
operation, separate from unregistering.

## Seeing your apps

The `/apps` page in käsi's web UI lists every registered app with its URL, its status,
and its recent logs. The division of labour is clean: käsi **collects** the apps (the
registry, and running them under systemd) and **surfaces** them (the URLs and this
page); exe.dev **exposes and protects** them (the port, the TLS, the auth). Nobody's
job overlaps.

Under the surface it's the same split as the rest of käsi: the registration — name,
port, start command, operations — is a directive in the event log, so the set of apps
rebuilds by replay. The code and data are files in the store, outside the log, exactly
like a memory's raw content versus its `remember` directive.

## The agent uses apps

An app has two front doors onto the same operations. The **UI** is yours — you reach
it through exe.dev's proxy, behind the login. The **operations** are the agent's — it
reaches them on `localhost`, inside the auth boundary, while it works a task. The form
you submit and the call the agent makes hit the same `POST /receipts`; they just
arrive by different doors.

The agent learns what apps exist the same way it learns everything else about a run —
from a file laid into `in/`. Each app describes itself in an `app.json` in its own
directory; käsi aggregates those into a single `apps.json` it lays into every run
beside `MEMORY.md`, one entry per app, its local URL and its operations:

```json
{
  "accounting": {
    "url": "http://localhost:3412",
    "operations": [
      { "method": "POST", "path": "/receipts", "purpose": "file a receipt",
        "payload": "vendor, date, amount, currency, category" },
      { "method": "GET",  "path": "/export",   "purpose": "download the ledger as CSV" }
    ]
  }
}
```

That's the whole interface. The agent reads `apps.json`, sees what each app can do, and
calls it on `localhost` — no hardcoded ports, no route table, no special email
plumbing. All mail still arrives at käsi's one address and becomes a task the normal
way; using an app is just something the agent does *inside* that task.

### Operations and policy

Two things decide whether a receipt lands in the accounting app, and each lives in a
place käsi already has:

- **`apps.json` is the *how*** — the operations an app exposes. It comes from the app's
  registration, so it's always current.
- **Memory is the *when*** — "forwarded receipts get filed in accounting; business ones
  filed, personal ones just replied to." You teach it once, and it's provisioned into
  every future run.

The agent bridges the two, per email, with judgment. That's what makes "sometimes"
work: whether *this* receipt is a business expense is exactly the call an LLM should
make and a rules engine shouldn't.

### The receipt, end to end

1. The agent builds `accounting`: a UI for you, plus operations (`POST /receipts`,
   `GET /export`) described in its `app.json`. `kasi app add` records them; käsi runs
   the app and starts listing it in every run's `apps.json`.
2. You forward a receipt to käsi and, once, say "file this in accounting." The run
   reads `apps.json`, sees the `receipts` operation, pulls the fields off the receipt,
   POSTs them on `localhost`, and replies "filed." It writes the policy to memory.
3. Next time you just forward the receipt. Memory (when) plus `apps.json` (how) mean
   it's filed automatically, no note needed — and the business-vs-personal nuance is a
   memory the agent applies case by case.
4. You open the accounting UI through the proxy whenever you want to see or correct
   what's there.

No new email pipeline, no per-app inbox, no routing table — just the agent, doing a
task, reaching for a tool it was handed in `apps.json`.

## Limitations

- **Trust is the VM, not the app.** An app is arbitrary code the agent wrote, running
  as käsi's user. The boundary is the private host: apps are never public by default
  and are never handed käsi's secrets, but they aren't yet sandboxed from each other
  or from the rest of the store — one app can read another's files. Per-app isolation
  is a later hardening step. Until then, the audience is you (and whoever you've given
  VM access), which is what keeps this reasonable.
- **A port in the URL, from a fixed band.** Each app gets one port in exe.dev's
  forwarded 3000–9999 range and is reached as `vm.exe.xyz:PORT`. Apps own their root,
  which is the nice part, but the URL carries a port number rather than a friendly
  name, and the band caps how many apps can run at once. Friendly per-app hostnames
  (via custom domains) are a later refinement.
- **The port must be reachable by the proxy.** The app has to listen where exe.dev's
  forwarder can see it — the VM's interface, not just loopback — or the URL won't
  answer. käsi passes the port; binding it correctly is the app's job.
- **systemd, with linger.** Apps run under `systemctl --user` so they survive logout
  and reboot. That ties the feature to a systemd host — right for the exe.dev box, not
  portable to a machine without it.
- **One process, kept up — not scaled.** käsi runs a single instance per app and
  restarts it on crash. No load balancing, no multiple instances, no zero-downtime
  rollout. This is for small, personal, adhoc apps, not production services.
- **Apps don't talk.** An app can't send you email or start a task — only an agent
  does, during a task. A proactive nudge ("you're missing a receipt for the Stripe
  charge") isn't something the app can do on its own; it's a scheduled-task concern,
  where an agent inspects the app and emails you. Apps are tools, not participants.
- **No managed build.** käsi runs your start command; installing dependencies and
  building is the agent's job at build time, in the app directory. A runtime the box
  doesn't have is the agent's problem to solve before registering.
