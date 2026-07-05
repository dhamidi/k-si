# 06 — Secrets

käsi needs credentials — a Fastmail API token, agent/API keys, and whatever a
task's tools require (payment provider keys, etc.). These are handled by one
rule: **secrets are references everywhere except at the moment of use.**

## The model

- Secrets live in a **separate SQLite database file** from the main database
  ([03](./03-persistence.md)). Physical separation keeps plaintext out of the
  message log, inbox/outbox, and archives, and lets the secrets file carry
  tighter file permissions and its own backup policy.
- A secret is addressed by a **`secret://` URL**. The URL is an opaque
  reference, safe to store and log.
- The **runtime resolves** a `secret://` URL to its plaintext value **only inside
  a command interpreter**, at the edge, at the instant an effect needs it. The
  resolved value is used and discarded; it is never written back into the model,
  the log, or any message.

```
secret://fastmail/api-token
secret://agent/anthropic-key
secret://route/pay/stripe-key
```

The path names a namespace and key; the meaning of the namespaces is convention
(`fastmail`, `agent`, per-route buckets like `route/pay`).

## Why a URL, and why the runtime resolves it

- **Handlers stay pure and safe to log.** A handler may put
  `secret://fastmail/api-token` into a command's payload; that command (and the
  message that produced it) can be logged and replayed with no plaintext
  exposure ([01](./01-architecture.md)).
- **Resolution is a single, auditable choke point.** Exactly one place — the
  resolver, called from effects — turns references into plaintext. That is where
  access can be logged, scoped, and reasoned about.
- **Replay never touches plaintext.** Because effects are suppressed during
  replay ([01](./01-architecture.md)), resolution never even runs during replay.
  A log replayed on any machine reconstructs full state without needing the
  secrets file at all.

## Secrets database schema (illustrative)

```sql
CREATE TABLE secret (
  namespace  TEXT NOT NULL,      -- 'fastmail', 'agent', 'route/pay', ...
  key        TEXT NOT NULL,      -- 'api-token', 'anthropic-key', ...
  value      BLOB NOT NULL,      -- the credential (encrypted at rest, see below)
  updated_at TEXT NOT NULL,
  PRIMARY KEY (namespace, key)
);
```

`secret://<namespace>/<key>` maps to the `(namespace, key)` row.

### Encryption at rest

Values are encrypted with a key that is **not** stored in either database —
supplied to the process by its environment (an exe.dev-provided env var or a
file only the process can read). The database file therefore leaks nothing
useful on its own. Resolution decrypts in memory per use. (If a hardware/OS
keystore is available on the host it may back this, but the baseline is a single
process-level key held outside SQLite.)

## Resolution flow

Where a `secret://` URL turns into a value:

1. A handler decides an effect needs a credential and emits a command whose
   payload contains the `secret://` **URL** (not the value).
2. The command interpreter, before performing the effect, passes the payload
   through the **resolver**, which replaces each `secret://` URL with its
   decrypted plaintext **in a local copy** used only for this effect.
3. The effect runs (send via JMAP, spawn a harness with the key in its env,
   call a payment API).
4. The plaintext is dropped when the effect returns. Nothing plaintext is
   emitted back as a message.

For **agent runs** specifically ([05](./05-agents-and-tasks.md)), the resolver
injects resolved values into the harness's environment/config at spawn time. The
agent gets the credential it needs; the workspace files, the model, and the log
never contain it. Resolved secrets must not be written into `in/` or `out/`.

## Managing secrets

A human types a plaintext credential in exactly two places, both on the web,
never in email:

- the **settings UI** ([08](./08-web-ui.md)), for standing credentials (the
  Fastmail token, agent keys, per-route provider keys);
- an agent-raised **UI request** form ([05](./05-agents-and-tasks.md),
  [08](./08-web-ui.md)), when an agent needs a secret mid-task — the user provides
  it on the request page rather than pasting it into a reply.

Both write to the secrets database (encrypting on write) and hand back a
`secret://` URL. From that point the two paths are identical: the value is a
reference everywhere, resolved only at the edge. In particular, a secret provided
through a request never enters the `answer-ui-request` message, the log, the
model, or a workspace file as plaintext — the web edge stores it and passes on
only its `secret://` URL ([05](./05-agents-and-tasks.md),
[03](./03-persistence.md)). The UI otherwise only ever displays that a secret
*exists*, never its value.

Because the UI has no in-app auth, this depends entirely on the host keeping the
deployment private ([08](./08-web-ui.md)) — a deliberate trade recorded here so it
is not forgotten.

## Invariants

1. **No plaintext in the main database.** Not in the log, inbox, outbox, or
   archives — only `secret://` references.
2. **No plaintext in messages or commands after resolution.** Resolution output
   never becomes a runtime message.
3. **No plaintext in workspaces.** Agents receive secrets via environment/config
   at spawn, not via files.
4. **One resolver.** All resolution goes through the single choke point so access
   is uniform and auditable.
5. **The decryption key lives outside SQLite**, supplied by the host
   environment.
