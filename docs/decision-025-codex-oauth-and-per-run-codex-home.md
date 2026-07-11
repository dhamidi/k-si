# Decision 025 — Codex OAuth sign-in, and a per-task CODEX_HOME

> **Revision (adversarial review).** The first cut made `CODEX_HOME` *per-run* and tore
> it down after every `Wait`. That broke resume: codex stores its session rollouts under
> `$CODEX_HOME/sessions/`, so a fresh home each turn left `codex exec resume`/`--last`
> with nothing to continue on turn 2. The home is now **per-task and persistent** at
> `$STATE/codex/<taskID>`, reused across the task's turns and reaped only when the task
> ends (or by a boot sweep). The sections below are written to that corrected model; §6
> and §7 record the review's other fixes (audited rotation, reserved-ref hardening).

## Context

[decision-024](./decision-024-harness-selection-and-generalized-session-identity.md)
made the worker harness selectable and let a Codex run carry its own session, but
it left three things to follow-on work and made one assumption that the live box
disproved.

- **The credential.** decision-024 assumed `codex` is authenticated the way `claude`
  is — a login already sitting in `~/.codex/auth.json` on the host. That is true of
  the operator's own box, but it is not something käsi can *set up*: there was no way
  to sign a fresh deployment into a ChatGPT subscription through käsi, and a Codex run
  read whatever host login happened to exist, with no isolation between runs and no
  managed home.
- **Native skills.** Codex discovers skills under its own `CODEX_HOME/skills/`, which
  is a different mechanism from käsi's provisioned Claude skills
  ([decision-011](./decision-011-nested-agent-output.md), the agent-authored skills of
  decision-009/010). A Codex run saw none of the task's learned skills.
- **Session identity.** decision-024 §4 spoke of Codex *minting* its own session id
  and returning it from `Start`. Running the real binary showed the id is not minted
  by käsi at all — it is **announced by codex** on the first stdout line as
  `{"type":"thread.started","thread_id":"…"}`, which käsi must **harvest** off the
  stream rather than generate.

All three want the same missing primitive: a private place, per run, that holds the
credential and the skills for exactly one Codex turn. And the credential itself must
honor decision-004 to the letter —
[decision-004](./decision-004-secrets-are-written-at-the-web-edge-resolved-at-the-agent-edge.md):
a secret's plaintext appears only at an edge, never in the model, the log, a message,
or a harvested/archived `out/` file, **and never as an OS env var** (which would leak
it through `/proc/<pid>/environ`).

## Decision

### 1. A per-task persistent CODEX_HOME is the primitive

The start-agent-run effect — the single choke point that already materializes secrets
at the edge — gives each real Codex run the task's **private, persistent `CODEX_HOME`**
at **`$STATE/codex/<taskID>`**, created idempotently (`os.MkdirAll`) at run start and
**reused across every one of the task's turns**. It lives beside the databases and
workspaces under `$STATE`, **outside the task workspace and `out/`**, so the manifest
harvest and the archive never touch it (decision-004,
[decision-019](./decision-019-out-is-a-per-turn-outbox.md)). It is the single landing
site for two things a Codex process needs and nothing else provides: the operator's
credential as a `0600` `auth.json`, and the task's learned skills linked into
`CODEX_HOME/skills/<name>/`.

**Why per-task, not per-run.** codex keeps its session rollouts under
`$CODEX_HOME/sessions/`. `codex exec resume <id>` and `codex exec resume --last` select
from *that store*, not from the run's cwd. If the home were minted fresh each turn and
removed after `Wait` (the first cut), turn 2's resume would find an empty `sessions/`
and could not continue the real thread — the whole session-identity mechanism of §5
would be dead on arrival. A stable per-task home is what makes resume real: the rollout
codex wrote on turn 1 is still there on turn 2. The credential (`auth.json`) is
*re-materialized/refreshed* each turn so a rotation elsewhere is picked up, but the
directory and its `sessions/` persist. `$STATE` is threaded to the effect and the
task-finish path as a plain config string (`CodexHomeRoot`, like `ControlURL`) — no
effect reads the model to find it.

Only the **directory path** rides the run environment (`CODEX_HOME=$STATE/codex/<id>`).
The credential blob itself — ~4 KB of `auth.json` — is written as a `0600` file inside
that dir and is **never** an environment variable. This is strictly better than an
env-var blob for decision-004: a `0600` file in a private state dir is not world- or
sibling-readable through `/proc/<pid>/environ`, does not appear in a child's inherited
environment, and is removed with the home at task-finish. The materialization is called
only when the resolved harness is the real Codex, so it is inert in every twin ring
(sim, recorded, recording) and cassettes are untouched.

`seedCodexAuth` prefers the reserved secret (below) and falls back to copying the
host's `~/.codex/auth.json` when no secret is stored — preserving today's
host-logged-in posture — and writes nothing when neither exists, so codex fails to
authenticate and the run records the error rather than käsi inventing a credential.
`seedCodexConfig` writes a **minimal** `config.toml` (once) rather than copying the
host's: copying it would bleed the operator's API-key/provider/MCP settings into a home
whose auth is the ChatGPT OAuth `auth.json`, and a stray API key would override the
OAuth login.

### 2. OAuth sign-in through käsi at a host-gated /codex surface

`/codex` is a new **host-gated** page
([decision-006](./decision-006-browse-ui-is-host-gated-no-app-tokens.md)) that signs a
deployment into a ChatGPT subscription. Connect shells `codex login --device-auth`
against a dedicated käsi-managed home, harvests the one-time **public** code and
verification URL, shows them, and on a successful poll reads the resulting `auth.json`
once at the web edge and hands it straight to the secrets store — the blob lives only
as that `Set`'s argument, then is dropped (decision-004).

The credential is stored under the **reserved reference**
`secret://codex/oauth/auth-json` — an ordinary decision-004 secret. It renders on the
`/secrets` list like any other reference, rotation is a re-`Set`, and sign-out is a
`Delete`. Crucially, **"connected" is defined as this reference being present in the
store** — there is **no new model field**. The page derives its entire state (signed
in / waiting / expired / disconnected) from the secrets store and the one in-flight
sign-in the server may hold; nothing about the connection is logged except the
name-only `record-secret-set` / `record-secret-removed` that
[decision-023 (secrets)](./decision-023-secrets-are-managed-at-the-edge-audited-in-the-log.md)
already emits for every reference.

### 3. No public callback — device-auth polls out

The sign-in needs **no inbound callback**: `codex login --device-auth` has codex
**poll OpenAI outbound** while the operator approves the code in their own browser,
out of band. That is what lets this surface exist at all. The
ForwardEmail-inbound-webhook reachability wall
([decision-023, delivery](./decision-023-delivery-mechanisms-are-configured-in-the-model.md))
— käsi sits private behind exe.dev's IAM edge, there is no second public port, and no
safe way to host a public HMAC-authenticated receiver — **does not apply**, because
there is nothing inbound to receive. `/codex` carries exactly the same host-gated
trust as `/secrets` and `/control/notify`: the operator is already authenticated by
the host edge, so the connect/poll/disconnect actions need no app token.

### 4. The sign-in home is dedicated because device-auth is destructive

Live finding on codex 0.142.5: `codex login --device-auth` is **destructive** — it
**wipes the target `CODEX_HOME`'s `auth.json` before the operator approves**, not
after. Pointing it at the host's `~/.codex` (or at a run's home) would therefore
destroy an existing login the instant the operator clicked Connect, even if they never
finished. So the sign-in runs against its **own dedicated, freshly-made temp home**,
seeded with a minimal `config.toml`; the credential is harvested from there and the
home is removed on close. A restarted sign-in closes the previous attempt's home, and
a failed or expired attempt tears its home down — the destructive wipe only ever hits
a throwaway directory, never a working login.

### 5. Session identity is harvested, not minted

Correcting decision-024 §4's mint assumption: the Codex adapter **tees the process's
first stdout line**, parses `{"type":"thread.started","thread_id":"…"}`, and returns
that harvested `thread_id` on the `Handle`. The seed session is used only if the
stream ends without ever announcing one. Everything downstream of decision-024 is
unchanged: the harvested id still differs from `sessionFor(taskID)`, so the run still
logs a `record-session`, and the next turn's `Resume` still reads `run.Session` from
the model and resumes the exact thread. käsi generates no session id for Codex; it
reads the one codex declares.

### 6. Teardown is at task-finish, plus a boot sweep

Because the home is now per-task, it is torn down where the **task** ends, not where a
run ends. The `archive-task` effect — the existing workspace-cleanup path that deletes
`task-<id>/` — also `RemoveAll`s `$STATE/codex/<taskID>`, so the `0600` credential never
outlives the task (decision-004). `finalizeCredentials` no longer removes anything; it
only writes back a rotation (§7).

A crash between a task finishing and its home being reaped, or a run that errors before
it registers, would otherwise orphan a home with a live credential in it. So `kasi
serve` runs a **boot sweep** after the model is replayed: it walks `$STATE/codex/`,
parses each entry as a task id, and removes every home whose task is **done or absent**,
keeping only those still in flight (present and not `Done`) so an interrupted task
resumes into its home (decision-015). No `0600` credential survives a restart it
shouldn't.

### 7. Rotation write-back is audited and connection-gated; the reserved ref is fenced off

`finalizeCredentials` persists a token codex rotated **mid-turn** back to the reserved
reference, but the first cut did so with an unaudited direct `Set` that fired even when
the operator had signed out (Deleted the ref) or when the run had fallen back to the
host `~/.codex`. Two fixes:

- **Connection-gated.** The write-back happens **only if the reserved reference still
  exists** — the same "connected" predicate §2 defines. A signed-out operator is never
  silently re-signed-in by a rotation, and a host-`~/.codex` fallback (which stored no
  reserved reference) never writes one. It also skips when the token did not actually
  change this turn.
- **Audited.** The write is routed through the same audited edge the `/codex` handler
  uses: `Set` the reference, then record the **name-only** `record-secret-set`
  (decision-023). A rotation at the agent edge now shows up in the audit exactly like a
  web sign-in — the reference name only, never the value (decision-004). This is the one
  place the harness emits a message, and it is safe for replay convergence precisely
  because it only ever runs on the real `*Codex`, which no test ring resolves.

Finally, the reserved reference `secret://codex/oauth/auth-json` is **fenced off from
the ordinary Flow-C secrets path**: `startAgentRunEffect` rejects it in `SecretRefs`
resolution, so an agent's web-form request can never resolve the OAuth blob into a
worker env var (which would put it on `/proc/<pid>/environ`). The credential reaches a
run **only** as the `0600` `auth.json` inside the private home, never as an environment
variable.

## Consequences

- **decision-004 is strengthened, not merely preserved.** The credential now has a
  first-class home that is a `0600` file in a private temp dir, path-only through the
  environment. There is no env-var blob anywhere and no plaintext in the workspace,
  `out/`, the model, the log, or a message — a stricter posture than the host-login
  assumption it replaces.
- **No new model surface.** "Signed in" is the presence of a reserved reference, so
  replay convergence holds trivially: no new field marshals, and the only log entries
  are the name-only secret-mutation records decision-023 already defines. The
  Claude/sim/recorded logs and committed cassettes stay byte-identical.
- **The twin rule holds.** The per-task home, the credential materialization, and the
  real device-auth launcher are all gated on the real Codex; the sim sign-in returns
  canned public values and a sentinel credential, so `t/web/codex-signin.test`
  exercises the whole connect → poll → harvest → sign-out loop without a live login.
- **The lifecycle is covered without a live codex.** The home/auth/finalize core is
  gated on `h.(*Codex)`, which no ring resolves, and the merge gate forbids `*_test.go`
  (the scripts are the tests). So the decision-004-critical behavior is factored into
  pure and package-internal seams (`CodexHomeDir`, `materializeCodexHome`,
  `RemoveCodexHome`, `SweepCodexHomes`, `finalizeCredentials`) and driven by
  `t/research/codex-lifecycle.test` via a `codex-lifecycle` vocab that calls
  `agents.VerifyCodexLifecycle`. It asserts reuse across resume turns (same home,
  `sessions/` survives), `0600` perms, no plaintext blob in env, the minimal config,
  teardown on task-finish, the boot sweep, and rotation-writeback-only-when-connected —
  without a subprocess or harness dispatch, so the twin rings stay byte-identical.
- **A run may fail closed.** With no stored secret and no host `auth.json`, a Codex run
  starts with an unauthenticated home and records codex's auth error — deliberate: käsi
  never fabricates a credential.
- **The box stayed logged out.** This decision was verified by building and running
  scenarios, never by a live `codex login` (which, per §4, is destructive).

## Related

- [decision-004](./decision-004-secrets-are-written-at-the-web-edge-resolved-at-the-agent-edge.md) — plaintext only at an edge; the rule the per-run `0600` home upholds and tightens.
- [decision-006](./decision-006-browse-ui-is-host-gated-no-app-tokens.md) — the host-gated posture `/codex` inherits, no app tokens.
- [decision-015](./decision-015-an-interrupted-run-resumes-not-orphans.md) — relaunch-exactly-once, which the harvested session keeps intact across a restart.
- [decision-019](./decision-019-out-is-a-per-turn-outbox.md) — the per-turn `out/` framing the transient home stays outside of.
- [decision-020](./decision-020-settings-are-typed-contributions-rendered-by-a-runtime-form-engine.md) — the settings module `/codex` sits alongside; the harness choice that selects Codex.
- [decision-023 (delivery)](./decision-023-delivery-mechanisms-are-configured-in-the-model.md) — the inbound-webhook reachability wall that device-auth's outbound poll sidesteps.
- [decision-024](./decision-024-harness-selection-and-generalized-session-identity.md) — the harness registry and session generalization this completes, correcting its mint assumption to a harvest.
