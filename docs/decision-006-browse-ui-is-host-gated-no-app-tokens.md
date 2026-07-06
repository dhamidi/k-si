# Decision 006 — the browse UI is host-gated, no app-layer tokens

**Status:** accepted (task/transcript views, stage 3)

## Context

The new pages — task list (`/tasks`), task detail (`/tasks/{id}`), the transcript
view, and the **Stop** action (a POST that emits `stop-agent-run`) — expose task
content and can halt a running agent. Every other capability URL in käsi (the
completion link, the Flow C request link) carries an unguessable per-request
**token** checked constant-time. Should these browse/mutate routes carry a token
too?

## Decision

**No.** The browse and Stop routes carry no token. They rely entirely on the host
boundary: käsi ships no app-layer auth ([08](./08-web-ui.md), [00](./00-vision.md)),
and the deploy sits behind the exe.dev edge, which authenticates every request to
the public port (a transparent auth proxy — see the deploy notes). Tokens stay
exactly where they are load-bearing: the **email-triggered** links (completion,
request), which must work unauthenticated from a phone's mail client and therefore
cannot lean on the host session.

## Rationale

A token on `/tasks` would be security theatre: it would have to be discoverable by
the operator (so, in the model or a bookmark), it protects content the same host
session already gates, and it adds a check with no adversary the host boundary
doesn't already stop. The real invariant is **operational**: the deployment must
stay private (host/IAM-gated). That is already recorded as load-bearing
([08](./08-web-ui.md), [06](./06-secrets.md)); these routes inherit it rather than
reinventing a weaker per-route scheme.

## Consequences

- `GET /tasks`, `GET /tasks/{id}`, the transcript route, and `POST …/stop` do no
  token check; they read/mutate straight through the one front door.
- **If the deployment were ever exposed without the IAM gate, these leak task
  content and expose Stop** — the same failure mode docs/08 already flags for the
  secrets form. Keeping the host private is the invariant that guards them.
- Only `link.Completion` / `link.Request` routes remain tokened.
