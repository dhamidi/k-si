# Decision 008 — web render is tested with an in-process `visit` vocab

**Status:** accepted (task/transcript views, stage 3)

## Context

Until now the web edge was tested only by simulating what a handler *emits*: the
`click` vocab reproduces the completion POST's message, `answer` the Flow C
submit. Nothing exercised the **GET/render path** — route → model read → View
struct → htmlc `RenderPage` → HTML. The browse UI is almost entirely that path
(task list, task detail, transcript), so shipping it untested would repeat the
mistake this session already paid for: passing a gate that never ran the real
code ([test-with-realistic-payloads] lesson, docs/13).

## Decision

Add a **`visit <url>`** scenario vocab that drives the **real** `web.Server`
in-process. It constructs the server over the scenario's live sim app and edges
(the same `content`/`work`/secrets the world already holds) and issues an
`httptest` request through `Server.ServeHTTP`, capturing the response. Reads
return the rendered HTML for `matches`/`contains` assertions; a companion form of
it issues the POST for the **Stop** action. So a scenario can assert what a page
actually renders from a given model — the whole GET path, offline and
deterministic — exactly as `click` covers the completion path.

## Rationale

Testing the emitted message (as `click`/`answer` do) proves the write path but is
blind to rendering — a broken template, a wrong model read, a missing field would
all pass. Driving the actual `Server.ServeHTTP` with `httptest` runs the genuine
route table, handlers, View construction, and htmlc render, with no network and no
browser, so it belongs in the merge gate. It also makes the transcript parser
(decision-007) testable end-to-end: `visit` a run's transcript page built from a
committed real transcript fixture and assert the turns appear.

## Consequences

- New `visit`/`stop`-style vocab in `cmd/kasi/vocab.go`, holding a `web.Server`
  built from `inst.world` (content, workspace, a sim secret writer).
- Web-render coverage becomes a reusable capability: every future page lands with
  a `visit` assertion, not just a message check.
- A committed real transcript fixture (captured from the deploy, like the Flow C
  spec) drives the transcript-render regression.
