# UI-request form-spec fixtures (Flow C)

Form specs an agent writes to `out/request.json` — the array of fields the web
form is generated from. Used by the Flow C scenarios via `[fixture ui-request/…]`.

- **`bank-login.json`** — a hand-authored minimal spec (a secret + a text field),
  the first Flow C regression (`t/research/ui-request.test`).
- **`live-sample.json`** — **captured from reality**: the exact spec a real Claude
  agent produced on the live deploy (2026-07-06), pulled verbatim from the
  `register-ui-request` log record. All five field types, a 3-option `choice`, and
  **underscore** field names (`sample_text`, `sample_secret`) — real agent output
  differs structurally from a hand-written spec, so it exercises the parser, the
  form, and the answer path against what an agent actually emits
  (`t/research/ui-request-live-spec.test`). The graduation loop (docs/13) made real
  for Flow C.
