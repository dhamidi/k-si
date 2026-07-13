# Decision 026 — removing a skill clears both homes (edge blob + logged registry)

## Context

The owner could author and provision skills, and browse them at `/skills`, but
there was no way to retire one. `docs/07-skills-and-tools.md` already promised
"retiring an agent-authored skill is done from the UI" — a promise the code never
kept. A skill that is wrong, obsolete, or authored by mistake stayed in the
collection forever and, worse, was re-provisioned into every new agent run.

The load-bearing constraint is that **a skill lives in two homes that converge
differently**:

- **Home #1 — the tar blob** (`skill` content table). The whole skill directory,
  tarred, keyed by unique `Name`. A mutable *edge* with no model, read LIVE (not
  replayed) by provisioning: `provisionSkills` (`agents/command_start_agent_run.go`)
  calls `e.Content.AllSkills()` at task start and lays every skill into the run's
  workspace. Because this read is an effect, deleting only the log event would leave
  the blob provisioned into every *future* run.
- **Home #2 — the `skills` registry model.** Light, content-free metadata rebuilt on
  replay from logged events, and what `/skills` renders via `skills.All`.

The asymmetry is a test trap: `model skills` / `visit /skills` read only Home #2, so
a model/visit-only assertion green-passes even if the blob delete were forgotten. The
blob home is observable only by delivering a *fresh* task and asserting nothing was
provisioned.

## Decision

A delete hits **both** homes, by **different mechanisms**, from the web handler:

1. **Home #1 is a real side effect** — a new idempotent `Content.DeleteSkill(name)`
   on the interface and both twins (`SQLiteContent`: `DELETE FROM skill WHERE
   name = ?`, no rows-affected check; `MemoryContent`: slice-out by name). Deleting
   an absent name is a no-op success, mirroring `secrets.Delete`.
2. **Home #2 is a logged directive** — a new `unregister-skill` event whose pure,
   copy-on-write, order-preserving reducer drops the entry by name via the existing
   `findName`, mirroring `memory/message_forget.go`. The removal sticks on replay only
   because the directive is logged and replayed in order after the earlier
   `register-skill`.

**Edge-first ordering** (mirrors `web/form_secrets.go` `deleteSecret`): the handler
calls `s.content.DeleteSkill(name)` first — on error it returns 500 with no event, so
model and store stay consistent — then `s.app.Send(NewUnregisterSkill{name})` (which
blocks until applied, so the redirected GET already shows the skill gone in both
homes), then 303 to `skills.index`. The name rides the URL path (like memory forget),
not a hidden field (secrets need a hidden `ref` only because a `secret://` ref carries
slashes); decision-004 secrecy does not apply — a skill name is safe to echo, log, and
route.

**Hard-drop, not a removing-status saga.** Apps use a three-hop remove
(decision-021) because an app owns an external systemd unit. A skill owns nothing
external, so the delete is a plain hard-drop: edge DELETE + one logged event, no
status field, no reconcile step. A later genuine re-author legitimately re-adds the
skill via `register-skill` (a fresh insert, not a tombstone flip).

**Inline, no confirm** (the owner's fork choice) — the row's Remove control is a
`<form method=post>` that deletes directly, like memory's Forget, with no confirm
page. The registry is a log, so a removal's *history* is retained; the body archive is
deleted outright, so secrets' "cannot be undone" copy would be off-tone here.

## Consequences

- **Twin rule:** `DeleteSkill` ships in both content twins, held by the `var _
  Content` compile-time assertions. The `unregister-skill` event is pure (no Cmd, no
  edge), so it carries no sim-twin obligation.
- **Effects suppressed on replay:** the blob DELETE runs once at the live edge at
  request time and is never re-run on refold — only the event replays.
- **Idempotent throughout:** a web double-POST, a 303-retry, or a replayed unregister
  after the blob is already gone are all harmless. A crash between a successful
  `DeleteSkill` and the `Send` self-heals: the registry may briefly list a skill whose
  detail 404s, and a re-delete converges.
- **The asymmetric two-homes assertion is the correctness proof.** `t/web/skills.test`
  drives `post /skills/pay-invoice/delete`, then asserts the registry is gone
  (`visit /skills lacks` + `model skills skills is ""`) AND that a freshly delivered
  task provisions nothing (`task 2 provisioned are ""`) — the sole guard on the edge.
- **House rules:** `store/` is reserved, so `DeleteSkill` is hand-written; the
  `unregister-skill` message contract is kit-scaffolded (`providers/kasi/skill-removal.kit`);
  reverse-route via `router.Path`; host-gated, no CSRF (decision-006).
