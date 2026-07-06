# Decision 003 — request links mirror the completion link, keyed by run id

**Status:** accepted (Flow C, stage 3)

## Context

The completion link is a capability URL `/tasks/{id}/done?token=<token>`: the id
locates the task in the model, the unguessable token is the authorisation
(constant-time compared), and the `link` package builds/parses it via a named
`dispatch` route ([04](./04-email.md), [08](./08-web-ui.md)). A UI request needs
its own capability link. Its token is minted inside the `mint-ui-request`
**effect**; the request's own `register-ui-request` log offset is not known until
that message is applied — a chicken-and-egg the completion link never hits
because a task's id predates its token.

## Decision

The request link is **`/requests/{id}?token=<token>`**, where **`id` is the
raising agent run's id** — `RunID`, which is `meta.Offset`, a globally-unique log
offset. The request record is keyed by that run id. This mirrors the completion
link exactly: id in the path, unguessable token in the query, constant-time
compared against the record.

## Rationale

`RunID` is globally unique and **already known at mint time** (it arrives on
`finish-agent-run`), and a run raises at most one request (it exits after
prepping), so run id is a natural, stable request identity — sidestepping the
chicken-and-egg of using `register`'s own offset. Mirroring the completion link
means **one** capability-link pattern: one `link.Request`/`link.ParseRequest`
shaped like `link.Completion`/`link.ParseCompletion`, one token check reused from
`finishTask`, one `click`-style test vocab (`answer`) reused from the completion
test path. Divergence from that pattern would be new surface for no benefit.

## Consequences

- `link/link.go` gains `RequestRoute`, `RequestPattern = "/requests/{id}"`,
  `Request(base, runID, token)`, `ParseRequest(url) -> (runID, token)` — mirrors
  of the completion helpers, reusing `TokenParam`.
- `web/server.go` registers GET (render form) and POST (submit) on
  `RequestPattern`; both validate the token against the request record in the
  model, like `finishTask`.
- The `answer` test vocab mirrors `click`: parse the request link, token-check,
  perform the web edge's I/O, emit `answer-ui-request`.
