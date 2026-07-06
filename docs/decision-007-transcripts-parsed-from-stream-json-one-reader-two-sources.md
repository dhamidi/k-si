# Decision 007 — transcripts: parse stream-json into turns, one reader over two sources

**Status:** accepted (task/transcript views, stage 3)

## Context

The transcript view ([08](./08-web-ui.md)) renders an agent run's session legibly:
user turns, assistant turns, tool calls and results. The harness stores the
transcript **verbatim** as the Claude CLI's `stream-json` JSONL
([05](./05-agents-and-tasks.md)) — never reformatted. Inspecting real transcripts
on the deploy, the event shapes are: `system/init`; `assistant` messages whose
`content` blocks are `text`, `thinking`, or `tool_use` (`{name, input}`); `user`
messages with `tool_result` (`{tool_use_id, content, is_error}`) or `text`; and a
trailing `result` event (`{subtype, is_error, duration_ms, …}`). And a run lives
in **two** places depending on state ([08](./08-web-ui.md)): a finished run in the
`archive` table (`kind='transcript'`), a running run in the workspace
(in-progress, still being written).

## Decision

A small `transcript` package with a **pure** parser: `Parse(b []byte) []Turn`,
turning the JSONL into an ordered slice of typed turns — assistant text,
assistant thinking, a tool call (name + a one-line input summary) paired with its
result (and an error flag), user text, and a final result/status. Unknown event
or block types are skipped, not errored (the format is an open set we don't own).
The web handler does the **sourcing**: a finished run's bytes from
`Content` (the archived transcript), a running run's from `Work.ReadTranscript` —
same parser, same view, either way. The view renders turns structurally
(role-labelled blocks, tool calls with their output, thinking dimmed/collapsed) —
structure before style.

## Rationale

Keeping the parser pure and edge-free makes it trivially testable (bytes → turns,
asserted through the render) and keeps I/O at the edge where the two sources
differ. Skipping unknown shapes means a Claude CLI format bump degrades to
"rendered a bit less," never a crashed page. One reader over both sources is what
lets the same page show a live run and a finished one — the docs/08 requirement —
without branching the view.

## Consequences

- New `transcript/` package (`Parse` + `Turn`), pure, imported by `web/`.
- `web.Server` gains the `workspace.Workspace` edge (it already has `Content`) so
  it can read an in-progress transcript.
- The view renders from `[]Turn`; live-update (a running run) is a
  self-refreshing Turbo **fragment** of the same turns, degrading to a manual
  refresh with no JS (a fragment `view`, per [08](./08-web-ui.md)).
- `thinking` is shown dimmed/secondary, not hidden — legibility over polish.
