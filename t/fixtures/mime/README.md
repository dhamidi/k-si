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
