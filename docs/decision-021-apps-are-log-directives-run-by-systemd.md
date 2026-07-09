# Decision 021 — an app is a log directive; systemd runs it; käsi only reconciles

## Context

feature-apps.md: the agent writes a small web app into the store, hands it to
käsi with one command, and käsi keeps it alive on its own port behind exe.dev's
private front door. An app is "a directory, and a command that starts it" that
reads `$PORT` and serves. käsi **collects** apps (a registry, and running them
under systemd) and **surfaces** them (a `/apps` page); exe.dev **exposes and
protects** them (the forwarded port, the TLS, the login).

Two forces shape the design, and both are already load-bearing elsewhere in
käsi:

1. **The registry must rebuild by replay.** Which name holds which port, which
   units should exist — that is käsi's state, so it belongs in the event log, not
   a side table (decision-018's rule: state that must survive a restart is a
   logged directive). The *processes*, by contrast, are the machine's: systemd
   keeps them up across a käsi restart.
2. **The process is an edge, not a reducer concern.** Writing a systemd unit and
   starting it is I/O with the world — it runs live and must be suppressed on
   replay (docs/12), exactly like sending mail.

## Decision

**An app is a `register-app` directive in the log; a new `apprunner` edge makes
the machine match it; a reconcile subscription closes the gap crash-safely.**

### The registry is a model slice (the `apps` module)

`register-app` (add, or replace-in-place on re-add, keyed by name like a memory)
and `unregister-app` (mark for removal) are the only log directives. The `apps`
model is a slice of `App{Name, Port, StartCmd, Operations, URL, Status}` where
`Status` is `registered → running → removing`. Operations are stored as the
**raw `app.json` bytes**, unparsed — the store-raw / derive-on-replay rule
(decision-018, the memory feature): the agent edge re-parses them, käsi never
freezes a parsed value in the log.

### The runner is an edge (`apprunner.Runner`)

`Install/Start/Stop/Remove/Status/Logs`, every method idempotent. The real
adapter writes a `systemctl --user` unit (`kasi-app-<name>.service`) with linger
on — so apps survive logout and reboot and systemd, not käsi, restarts them on
crash — with `WorkingDirectory` the app's store dir, `Environment=PORT=<port>`,
and the start command run through a login shell so the app's runtime resolves on
PATH. The sim twin records the same unit state in memory so scenarios converge
without touching the host.

### Reconciliation, not fire-and-forget (decision-013, mirroring the outbox)

`register-app` records intent and returns NO command. A pure `apps-reconcile`
subscription is the sole launcher (decision-015): one source per app whose unit
does not match its desired status. `registered` ⇒ emit `run-app` ⇒
`install-app-unit` effect (`Runner.Install`+`Start`) ⇒ `mark-app-running` flips
the status; `removing` ⇒ `retire-app` ⇒ `remove-app-unit` ⇒ `mark-app-removed`
drops the entry. Because status only leaves `registered`/`removing` when the
effect's success is recorded in the log, a crash that loses an in-flight
`systemctl` is rebuilt by replay and the source fires again — and Install/Remove
are idempotent, so the re-fire is safe. This is the exact shape of
`email/subscription_outbox_reconcile.go`.

### Registration is a control command (the twin of `kasi notify`)

`kasi app add <name>` / `rm <name>` is a thin client: it reads the `KASI_*` env
already in the run and POSTs to the host-gated `/control/app` endpoint
(decision-006), which validates the per-run token constant-time against the live
run and injects the directive — a sibling of `kasi notify` and `/control/notify`
in every respect. The endpoint assigns the port from the registry
(`apps.FreePort`, the forwarded 3000–9999 band) — reusing an existing name's
port on re-add — and records it in the directive, so replay reconstructs the same
map without re-allocating.

### The `/apps` page collects; exe.dev protects

Host-gated, no token, read-only (a view never writes; registering is `kasi app`'s
job). It renders the registry from the log and reads each app's live status and
recent journald lines through the same `Runner` edge that keeps them up — the
registry is käsi's, the liveness is the machine's — degrading to "unknown" when
the edge can't answer, so a browse page never fails on a machine it can't reach.

## Reconciled with reality: apps are direct children of the store

feature-apps.md wrote `./store/apps/<name>/`, but the store on the live machine
keeps apps as **direct children** — `store/accounting/`, `store/finnair/`. The
implementation follows reality: an app `<name>` lives at `store/<name>/`, which
is the `apprunner` root and the unit's `WorkingDirectory`. No `apps/` subdir, no
move of existing apps.

## Consequences

- **A registry that survives restart, processes that never stopped.** käsi
  rebuilds name→port→unit from the log; systemd kept every app running meanwhile.
- **Crash-safe registration.** A crash between the append and the unit write is
  recovered by the reconcile source on the next boot; idempotent Install makes
  the retry a no-op-or-replace.
- **No proxy, no routing.** käsi never touches an app's traffic. exe.dev forwards
  the port and applies the login; the app owns its whole root.
- **Trust is the VM.** An app is arbitrary agent-written code running as käsi's
  user, private by default, never handed käsi's secrets, not yet sandboxed from
  the rest of the store (feature-apps.md's limitation stands).

## Deferred (a follow-up, not in this cut)

- **"The agent uses apps"** — aggregating each app's `app.json` into an
  `apps.json` laid into every run beside `MEMORY.md`, so the agent can call an
  app's operations on `localhost` while it works a task. That is the provisioning
  half of feature-apps.md; it threads apps through the `start-agent-run` payload
  the way memory already is, and is its own decision.
- **`kasi app ls/logs/restart`** beyond `add`/`rm` — operational conveniences over
  the same runner; the `/apps` page already surfaces status and logs.

## Coverage

- `t/apps/register-and-run.test` — a run registers an app through the real
  `/control/app` endpoint; the reconcile loop drives the sim Runner to
  `running`; `/apps` lists it up; a later turn removes it and the registry drops
  it.

## Related

- `apps/` (the module), `apprunner/` (the edge: contract, systemd adapter, sim
  twin), `web/browse_apps.go` + `web/view_apps.*` (the page),
  `cmd/kasi/app.go` (the client), the `/control/app` handler in `web/server.go`.
- [decision-013](./decision-013-post-finish-effects-are-reconciled-not-fire-and-forget.md)
  — reconciliation over fire-and-forget.
- [decision-015](./decision-015-an-interrupted-run-resumes-not-orphans.md) — the
  sole-launcher subscription.
- [decision-018](./decision-018-poll-cursor-in-the-log.md) — record the derived
  edge value (here, the port) in the log.
- [decision-014](./decision-014-notifications-are-fire-and-forget.md) /
  feature-notifications.md — the `kasi notify` control-command twin.
