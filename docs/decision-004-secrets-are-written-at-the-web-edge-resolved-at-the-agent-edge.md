# Decision 004 — secrets are written at the web edge, resolved at the agent edge

**Status:** accepted (Flow C, stage 3)

## Context

Flow C's whole point is to collect a secret (or file) **without it touching
email, the log, or the model as plaintext** ([05](./05-agents-and-tasks.md),
[06](./06-secrets.md)). Two I/O moments: writing the submitted secret, and later
handing it to the resumed agent. The `Secrets` edge interface deliberately
exposes only `Resolve`; `Set` lives on the concrete `*SQLiteSecrets` as a
management operation.

## Decision

**Write at the web edge.** The POST handler in `web/` does the I/O the docs
assign to it: it stores uploaded files via `Content.AddArchive` and writes each
`secret`-typed field via `SQLiteSecrets.Set` to `secret://task/<taskID>/<field>`,
then emits `answer-ui-request` carrying **only references** — archive ids,
`secret://` URLs — plus the non-secret text/choice values (which are not secrets
and may travel as plaintext). `web.Server` gains two dependencies: a secrets
**writer** (`Set`) and the content store. No new runtime edge is added for
writing secrets.

**Resolve at the agent edge.** The `spawn-agent-run` (resume) effect in `agents/`
resolves each `secret://` reference into the harness **environment** at the
instant of use: `agents.Edges` gains a `Secrets` resolver (`Resolve`), and
`Harness.Start`/`Resume` gain an environment parameter. Plaintext is materialised
only inside the harness process's env, never in a message, the log, the model, or
a workspace file.

## Rationale

This keeps all I/O at the edges and the pure core reference-only
([01](./01-architecture.md)), and reuses the existing `archive` + `secrets`
machinery rather than inventing storage. Writing at the web edge matches
[08](./08-web-ui.md) ("the web edge does all the I/O"); resolving at the agent
edge matches [06](./06-secrets.md) ("resolved into environment/config at the
edge, plaintext never enters the model or log"). The **secret-sentinel** standing
invariant (SimSecrets hands out sentinels) proves no plaintext leaks into the
log.

## Consequences

- `web.Server` gains a `secretWriter` (interface: `Set(url, plaintext) error`) +
  `store.Content`; wired in `NewServer` from `serve.go`'s `*SQLiteSecrets` and
  content store.
- `agents.Edges` gains `Secrets secrets.Secrets`; `Harness.Start`/`Resume` take
  an `env map[string]string`; the sim harness records it so scenarios can assert
  the resolved value arrived. `answer-ui-request` and `spawn-agent-run` carry
  `secret_refs` (env-var name → `secret://` URL).
- `secret://task/<taskID>/<field>` is the reference scheme for task-collected
  secrets.
