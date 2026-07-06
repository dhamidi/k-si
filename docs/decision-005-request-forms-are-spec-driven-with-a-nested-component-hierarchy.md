# Decision 005 — request forms are spec-driven, structure before style

**Status:** accepted (Flow C, stage 3)

## Context

The agent authors a **form spec** (fields, each `name` / `label` / `type` ∈
`text` `longtext` `choice` `file` `secret` / `required`); the web must render a
page from it and accept the answer. käsi's UI is mobile-first, semantic, and
progressively enhanced ([08](./08-web-ui.md)). The build brief: **a rich
component hierarchy that matches the information hierarchy; small screens first;
correctness of structure over prettiness — it can be styled later.**

## Decision

One page, `view_request`, renders any request from its spec — no bespoke page per
request type. The **component hierarchy mirrors the information hierarchy**:

```
view_request (page)            the request: why the agent needs input
├─ request_summary             the agent's message / the ask (leads the page)
├─ request_form                the single-column form
│  └─ request_field  (×N)      one per spec field: label + control + error
│     └─ (control by type)     text→input · longtext→textarea · choice→select
│                              · file→file input · secret→masked input
└─ request_answered            the closed/confirmation state once answered
```

The submit binds **dynamically from the spec** — a spec-driven `RequestForm`,
not a static per-form struct — since the fields are known only at runtime. Markup
is semantic, single-column, and **black-and-white / unstyled beyond layout**;
nothing depends on color or client-side scripting. Prettiness is deferred.

## Rationale

The information hierarchy is *request → fields → one field's control*; the
component tree matches it one-to-one, which keeps each piece small, legible, and
independently reusable (a `request_field` renders in isolation). Spec-driven
rendering means the agent describes *what* it needs and the page is generated
from that description — the mechanism stays inside the hypermedia, message-sourced
design with no endpoint per request type ([08](./08-web-ui.md)). Small-screens-first
single column is the dominant case (a tap from an email on a phone).

## Consequences

- `web/view_request.vue` + `view_request.go` (page, via `RenderPage`), composing
  `request_summary.vue`, `request_field.vue`, `request_answered.vue` sub-components.
- The `view` kit generator scaffolds the page; the sub-components are authored,
  and the **spec-driven field component** is a new reusable pattern to capture in
  the kit provider once proven.
- A dynamic `RequestForm` (spec + request → bound values/errors, then web-edge
  I/O) rather than a `flag.Value` static form; documented as the
  "dynamic/spec-driven form" companion to the static form object ([15](./15-tactical-patterns.md)).
- Styling is intentionally minimal now; a later pass adds visual design without
  touching structure.
