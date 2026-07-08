# Decision 019 — out/ is a per-turn outbox; a follow-up never re-sends the old reply

## Context

A task is a multi-turn conversation: each inbound message spawns an agent run in
the task's workspace, the run writes `./out/reply.txt` (plus attachments), and the
harvest turns `out/` into an emailed reply. Whether a reply is sent is gated on
`hasReply(OutManifest)` — literally "is `reply.txt` present in the run's `out/`
listing?" — and `assemble-reply` reads the body from `out/` at harvest time.

`out/` was **never reset between turns**. The interface even documented `WriteOut`
as "appends across turns." So a `reply.txt` written on turn 1 stayed on disk. On a
follow-up turn where the agent wrote no new reply — e.g. a resumed run that decided
the task was already finished and stopped — the stale turn-1 `reply.txt` was still
in the manifest, so the harvest re-sent it verbatim.

Observed on the live system (task 3827, "Canadian Land Grants"): three turns, three
sent replies with **byte-identical** bodies. `out/reply.txt` was last written on
turn 1; turns 2 and 3 re-mailed it. The user received "the same email twice." The
agent's turn-3 transcript: *"The task is complete… No further action is needed.
Stopping."* — it never rewrote the reply, and käsi sent the old one anyway.

Two things had to be true for this to ship:

1. **`out/` accumulated across turns** — the deliverable box was cumulative, not
   per-turn.
2. **The sim harness hid it.** The sim reported only the *current* turn's writes as
   its manifest (`manifestOf(outParts)`), while the real Claude harness walks the
   whole `out/` directory (`c.manifest`). So the sim never re-sent a stale reply —
   the gate could not see the bug. A twin-fidelity hole (docs/12).

## Decision

**`out/` is emptied at the start of every turn.** A new `Workspace.ResetOut` clears
the box, called from the `start-agent-run` effect — the single choke point every
turn passes through (first turn, email reply, and web-form answer alike) — before
the harness runs. So `out/` afterwards holds exactly what THIS turn produced:

- a turn that writes no `reply.txt` sends nothing (no duplicate);
- a fresh `reply.txt` with no attachment does not drag along a prior turn's
  attachment.

`in/` is deliberately **not** reset — prior context still accumulates there, and the
resumed Claude session still carries the full history.

Two supporting changes:

- **Resume prompt (behavioural).** `Start` and `Resume` sent the identical worker
  prompt; a resumed session already holds the prior "I'm done" transcript, so the
  agent tended to re-affirm completion. `Resume` now leads with a preamble: a new
  message has arrived, this is a fresh turn, act on `./in/body.txt` now and always
  write a fresh reply. This makes the agent do the follow-up work instead of
  bailing — the other half of what the user saw.
- **Twin fidelity.** The sim harness now builds its manifest from the whole `out/`
  box (via `Files`), mirroring the real harness's directory walk. This is what lets
  the sim reproduce a stale re-send, so the gate guards it. With `ResetOut` keeping
  the box to one turn, the accumulated manifest equals the per-turn manifest for a
  well-behaved run, so existing scenarios are unchanged.

## Consequences

- **No stale re-send.** A follow-up turn emails only what that turn wrote.
- **Silence is possible, and correct.** If the agent writes no reply, käsi sends
  nothing rather than a duplicate — so the resume prompt matters: it keeps the
  agent from silently producing nothing on a real follow-up.
- **The deferred harvest reads `out/` after the run.** `ResetOut` runs at the NEXT
  turn's start, which only happens after a new stimulus. In the live loop a turn's
  harvest jobs drain (Settle) before the next stimulus is processed, so a later
  turn's reset never races a prior turn's harvest; and a crash cannot leave a
  harvest pending while a same-task follow-up is already in flight (the follow-up
  needs a stimulus the dead process never handled). A crash-resumed run (decision-
  015) is relaunched WITHOUT a new inbox message, so it keeps its own partial
  `out/` — the reset is tied to a turn, not to a bare relaunch.
- **Memory/skills unaffected.** Forget is an `in/memory/` deletion, not an `out/`
  absence; remember, skills, and requests are gated on their own freshly-written
  `out/` files. Clearing `out/` per turn is correct for all of them.

## Coverage

- `t/mail/follow-up-no-stale-reply.test` — turn 1 replies with an attachment; turn 2
  writes nothing and NOTHING is sent (pre-019 it re-sent the turn-1 reply); turn 3
  writes a fresh reply with no attachment and the turn-1 attachment does not ride
  along. Verified to FAIL with `ResetOut` neutered — a real regression guard.

## Related

- [decision-015](./decision-015-an-interrupted-run-resumes-not-orphans.md) — a
  crash-resumed run keeps its partial `out/`; the reset is per-turn, not per-launch.
- [decision-017](./decision-017-multiplayer-threads.md) — multi-party threads make
  follow-up turns common, so a per-turn outbox matters more.
- `workspace/workspace.go` (`ResetOut`), `agents/command_start_agent_run.go` (the
  choke point), `agents/harness_claude.go` (`resumePreamble`), `agents/harness_sim.go`
  (manifest from the whole box).
