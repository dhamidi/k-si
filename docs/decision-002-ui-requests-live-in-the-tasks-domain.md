# Decision 002 — UI requests live in the tasks domain

**Status:** accepted (Flow C, stage 3)

## Context

Flow C spans several domains: an agent run raises a request (agents); the reply
carries a link (email); the record + status coordinate the task lifecycle
(tasks); the form and I/O happen at the web edge; secrets and archive are edges.
Where should the request **record and its `register-ui-request` /
`answer-ui-request` handlers** live?

## Decision

The `UIRequest` model and the `register-ui-request` / `answer-ui-request` /
`lay-in-answers` handlers live in the **tasks** domain — the request record hangs
off the tasks model as `Task`-scoped state (a `Requests` list). `mint-ui-request`
lives in **email** (it builds the capability link and needs `BaseURL` + the
workspace, exactly as `assemble-reply` does).

## Rationale

A UI request is a **phase of a task's lifecycle**, like an agent run — it
references its task and the run that raised it ([02](./02-object-model.md)), and
answering it resumes that task. The tasks domain already orchestrates the
cross-domain lifecycle: `create-task` emits `spawn-agent-run`;
`agent-run-finished` emits `assemble-reply`. Housing the request there lets
`register-ui-request` set `Task.status` directly and `answer-ui-request` return
`[lay-in-answers, spawn-agent-run]` — the operations tasks already owns — with no
extra cross-domain message plumbing. A separate `uireq` module would have to
round-trip task-status and agent-run changes back through messages tasks already
handles, adding indirection for a concept that is inherently task-scoped.

`mint-ui-request` sits in email because minting is a link/token concern
(`BaseURL`, the `link` package) that mirrors how `assemble-reply` already builds
the completion link there. The split is clean: **email mints (link + token),
tasks owns the record and the orchestration.**

## Consequences

- `tasks/model_task.go` (or a sibling) gains the `UIRequest` type; the tasks
  `Model` gains `Requests`.
- `register-ui-request`, `answer-ui-request`, `lay-in-answers` are registered in
  `tasks/module.go`.
- `mint-ui-request` is an email command/effect; it emits the tasks message
  `register-ui-request`. Cross-module message emission is normal (messages are
  global); only *edges* are module-scoped.
