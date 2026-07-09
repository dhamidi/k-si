# Decision 022 — the agent uses apps through a provisioned in/apps.json

## Context

An app (decision-021) isn't only something the user opens — its operations are an
API the agent can call while it works a task, so an app is a durable *tool* the
agent uses, not just a page. feature-apps.md: käsi aggregates each app's `app.json`
into a single `apps.json` laid into every run beside `MEMORY.md`, one entry per
app — its local URL and its operations — and the agent reads it, sees what each
app can do, and calls it on `localhost`. That closes "forward me the receipt" →
"file it in the accounting app": memory says *when*, `apps.json` says *how*, the
agent bridges the two with judgment.

The registration half (decision-021) already records the registry in the log and
keeps the units up. What was missing is the read side: handing a run the set of
apps it may call.

## Decision

**Running apps are provisioned into a run as `in/apps.json`, on the exact path
memory already travels.** The provisioning is a workspace-edge write inside
`start-agent-run` — the single choke point every run passes through, right beside
`WriteMemory` and the skills provisioning:

- `Workspace.WriteApps(taskID, []AppFile)` renders `in/apps.json` — a JSON object
  keyed by app name, each entry `{url, operations}` — and lays it in with `LayIn`'s
  overwrite semantics, so it is an ordinary in/ input (Files lists it, archival
  keeps it). Both the OS edge and its sim twin write the byte-identical
  `appsJSON` render, so a scenario sees exactly what a run on disk would (docs/12).
- The URL is the app's **localhost** origin (`http://localhost:<port>`, built with
  net/url) — the agent reaches an app inside the auth boundary, no proxy, which is
  the whole point of "the agent uses apps." The public `https://vm:port` URL is
  the *human's* door (the /apps page); the local one is the *agent's*.
- Operations ride through as the app's **raw `app.json` bytes**; the edge extracts
  the `operations` array, defaulting to `[]` when the file is absent or has none —
  a pure-UI app still gets a well-formed entry. Nothing pre-parsed is trusted
  (store-raw / derive-on-replay).

**Only running apps are provisioned.** `apps.Running(v)` filters the registry to
`Status == running` — a merely-registered or removing app isn't callable, so it
isn't advertised. The launch handler (`handleLaunchAgentRun`), which has the View,
reads `apps.Running(v)` into the `start-agent-run` payload exactly as it reads
`memory.All(v)`. The payload is a transient effect input, never logged, so it may
carry the raw operation bytes.

**The index appears only once there is something to list.** With no running apps,
`WriteApps` writes nothing and prunes any stale `in/apps.json` from an earlier
turn — so a run never sees an app that has since been removed. This mirrors
memory's own rule (`MEMORY.md` appears only once a fact exists) and keeps a
run's in/ box clean when apps aren't in play.

The worker prompt now tells the agent, next to what it knows about `MEMORY.md`,
to read `./in/apps.json` for the apps it can call on localhost.

## Consequences

- **An app becomes a tool the agent reaches for.** The same `POST /api/records`
  the user submits through the UI, the agent calls on `localhost` while working a
  task — two doors onto one operation, no per-app email plumbing.
- **Always current, never stale.** Provisioned fresh at every run's start from the
  live registry; a removed app drops out on the next run, a restarted app keeps
  its entry.
- **Suppressed on replay.** `WriteApps` is a workspace effect inside
  `start-agent-run`; replay folds the log and never touches the in/ box.
- **Memory is still the policy.** `apps.json` is only the *how*. Whether *this*
  receipt is a business expense filed in accounting is a memory the agent applies
  case by case — the LLM's call, not a rules engine's (feature-apps.md).

## Coverage

- `t/apps/provisioning.test` — register an app (turn 1, reconciles to running);
  a follow-up turn is handed `in/apps.json` carrying the app's name, its
  `http://localhost:` URL, and an operations array. Asserts via `task 1 input
  apps.json matches ...` and `task 1 inputs matches "*apps.json*"`.

## Related

- [decision-021](./decision-021-apps-are-log-directives-run-by-systemd.md) — the
  registration half this reads from; `apps.Running` filters that registry.
- feature-memory.md — the provisioning path mirrored here (`WriteMemory` /
  `MEMORY.md`, index-only-when-non-empty).
- `agents/command_start_agent_run.go` (WriteApps + localAppURL),
  `agents/message_launch_agent_run.go` (`apps.Running(v)` into the payload),
  `workspace/workspace.go` (AppFile, appsJSON), `apps/model_apps.go` (Running).
