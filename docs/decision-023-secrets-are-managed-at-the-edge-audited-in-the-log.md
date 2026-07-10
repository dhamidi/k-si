# Decision 023 — secrets are managed at the edge; their mutations are audited in the log, name-only

## Context

Secrets are käsi's one credential edge (docs/06, decision-004): a secret is a
`secret://namespace/key` reference everywhere except the instant of use, where an
effect Resolves it to plaintext, uses it, and drops it. The plaintext lives sealed
in a separate encrypted database and never enters the model, a message, a workspace
file, or the log. Until now the only way to manage secrets was the `kasi secret
set/ls` CLI and Flow C's per-request masked field — there was no page to SEE what
credentials exist or to rotate/remove one, and no delete anywhere.

The operator wants a UI. Two forces pull on the design:

- **decision-004** says a value is written at the web edge and NEVER re-rendered —
  so a management UI can show *that* a secret exists and *when* it was last set, but
  never its value, and it must never route a value through a URL, a template, a
  message, or the log.
- **Auditability** (the decision-018 stance: state that matters belongs in the log)
  says "which credential changed, when" should be a replayable fact — but the value
  must stay off the log.

## Decision

**The value lives at the edge; the mutation is audited in the log, name-only.** Two
layers that never mix a value into the log.

**1. The edge is the source of truth for what exists.** `secrets` gains
`Entries() ([]Entry, error)` — `Entry{Ref, UpdatedAt}`, references and last-set
times, never values — and `Delete(url)`, on both `SQLiteSecrets` and its
`SimSecrets` twin (the sim tracks a `stored` set so its management view equals the
SET secrets, matching SQLite; the leak-scan sentinel set is untouched). The
`/secrets` list is `Entries()`, so it is always accurate — it includes secrets set
through `kasi secret set` on the CLI, not just UI writes. `kasi secret rm` is added
for parity.

**2. Add / rotate / delete are edge operations, gated per decision-004.** The web
add form's value field is a masked control; the plaintext is read from the POST,
passed straight to `secrets.Set(url, plaintext)`, and dropped — it never enters a
View, a template, a URL, a message, or a re-render (`Set` is idempotent, so add and
rotate are one operation). Delete calls `secrets.Delete(url)`. This is the SAME
handler-side gate Flow C already uses, on the same edge — not a new trust path.

**3. Mutations are audited in the log, name-only.** A new `credentials` module logs
`record-secret-set` / `record-secret-removed`, each carrying ONLY the reference; the
reducer stamps `meta.Time` (the runtime clock, deterministic on replay) and appends
an `Event{Ref, Op, At}`. The value never reaches this module. `credentials.Recent`
renders a "recent changes" trail on the page. This gives the replayable "which
credential changed when" auditability the operator wanted, while the value stays
sealed at the edge.

**4. Two views, two sources — deliberately.** The live LIST comes from the edge
(accurate, includes CLI writes); the audit TRAIL comes from the log (the history of
UI mutations). They are not one source: driving the list from the audit log would
miss a CLI-set secret, and logging the list would put nothing new in the log the
edge doesn't already hold. Keeping them separate is what makes both true.

**5. Host-gated, no token (decision-006).** Same trust as the operator's shell,
which can already run `kasi secret`. Deleting a credential is destructive (removing
the Fastmail token stops sending), so the UI confirms a delete before applying it.

## Consequences

- **You can see and rotate credentials from the web**, without a redeploy or a shell
  — the value is write-only, never shown.
- **An audit trail exists and is replayable** — set/removed events by reference and
  time, reconstructed from the log, with no value ever on it.
- **A CLI-set secret still appears** in the list (edge-sourced) even though it left
  no audit event — an accepted asymmetry: the list is current state, the trail is UI
  history. A CLI mutation is audited by the shell, not käsi's log.
- **No new leak surface.** The sim leak-scan (SENTINEL) still guards every ring: a
  value that escaped an effect into the model, log, or a rendered page fails the
  gate. The scenario also asserts the plaintext never appears on the page.

## Coverage

- `t/web/secrets.test` — the list + nav entry; adding a secret lists its reference
  and logs a name-only `set` event while the page and log never contain the
  plaintext (an explicit `lacks` assertion plus the leak-scan); rotate keeps one
  row; delete removes it and logs a `removed` event; an invalid add re-renders with
  an error and logs nothing.

## Related

- [decision-004](./decision-004-secrets-are-written-at-the-web-edge-resolved-at-the-agent-edge.md)
  — the gate this reuses; why a value never reaches a View, a message, or the log.
- [decision-006](./decision-006-browse-ui-is-host-gated-no-app-tokens.md) —
  host-gated, no token.
- [decision-018](./decision-018-poll-cursor-in-the-log.md) — the auditability stance
  the name-only trail satisfies without logging a value.
- `secrets/` (Entries/Delete + the sim twin), `credentials/` (the audit module),
  `cmd/kasi/secret.go` (`rm`), `web/` (the `/secrets` surface).
