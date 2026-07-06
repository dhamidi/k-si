# MIME fixtures — captured from reality

These `.eml` files are real messages captured from the wire, not authored by
hand. The best parser test cases are the ones reality wrote: they carry the
headers, encodings and multipart nesting that actual mail clients emit, which is
exactly where a hand-written fixture would quietly diverge from what breaks in
production.

## Provenance

- **invoice-fwd-pdf.eml** — captured from the `kasi@decode.ee` inbox. A real
  Gmail forward of an E.ON invoice, carrying a PDF attachment. Its shape is
  `multipart/mixed[ multipart/alternative[text/plain, text/html], application/pdf(26425390718.pdf) ]`.
  This is the message that shipped broken (bug 2): the pre-fix, non-recursive
  `mime.readMultipart` never descended into the nested `multipart/alternative`,
  so the text body came out EMPTY and the PDF was dropped — yet a task was still
  created. Guarded by `t/mail/parse-invoice.test`.

## Growing the corpus from reality

New fixtures are captured, never authored. `kasi capture-inbox` reads the live
Fastmail inbox (read-only — JMAP `Recent`, no writes, no mail sent) and writes
each recent message to `<slug>.eml` here, where `<slug>` is a lower-kebab base
from the Subject plus a short hash of the Message-ID. The hash makes the mapping
deterministic: the same message always lands in the same file, so re-running
overwrites rather than duplicates.

`provenance.json` records each captured file → `{message_id, from, subject,
recipient, captured_at, structure}`, where `structure` is the top-level
Content-Type (the shape at a glance, e.g. `multipart/mixed`). It is
captured-from-reality metadata whose `source` is the live account. This is
ring-3 tooling: run it deliberately in an environment with credentials
(`kasi capture-inbox [-n N] [-state ./data] [-dir t/fixtures/mime]`); it is
never in the merge loop.
