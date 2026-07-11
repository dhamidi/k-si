# Decision 025 — Codex OAuth sign-in, and a per-run CODEX_HOME

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

### 1. A per-run transient CODEX_HOME is the primitive

The start-agent-run effect — the single choke point that already materializes secrets
at the edge — builds a **private, transient `CODEX_HOME`** for each real Codex turn
and tears it down after `Wait`. It lives in the OS temp dir, **outside the task
workspace and `out/`**, so the manifest harvest and the archive never touch it
(decision-004,
[decision-019](./decision-019-out-is-a-per-turn-outbox.md)). It is the single landing
site for two things a Codex process needs and nothing else provides: the operator's
credential as a `0600` `auth.json`, and the task's learned skills linked into
`CODEX_HOME/skills/<name>/`.

Only the **directory path** rides the run environment (`CODEX_HOME=/tmp/…`). The
credential blob itself — ~4 KB of `auth.json` — is written as a `0600` file inside
that dir and is **never** an environment variable. This is strictly better than an
env-var blob for decision-004: a `0600` file in a private temp dir is not world- or
sibling-readable through `/proc/<pid>/environ`, does not appear in a child's inherited
environment, and is removed with the home. The materialization is called only when the
resolved harness is the real Codex, so it is inert in every twin ring (sim, recorded,
recording) and cassettes are untouched.

`seedCodexAuth` prefers the reserved secret (below) and falls back to copying the
host's `~/.codex/auth.json` when no secret is stored — preserving today's
host-logged-in posture — and writes nothing when neither exists, so codex fails to
authenticate and the run records the error rather than käsi inventing a credential.

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
- **The twin rule holds.** The per-run home, the credential materialization, and the
  real device-auth launcher are all gated on the real Codex; the sim sign-in returns
  canned public values and a sentinel credential, so `t/web/codex-signin.test`
  exercises the whole connect → poll → harvest → sign-out loop without a live login.
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
</content>
</invoke>
