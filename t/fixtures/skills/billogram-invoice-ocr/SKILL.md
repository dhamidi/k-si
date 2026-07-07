---
name: billogram-invoice-ocr
description: >-
  Extract the payment details (OCR/reference number, bankgiro, IBAN/BIC, amount,
  due date, creditor) from a Billogram invoice link (billogram.com/l/... or
  /r/...). Use whenever a bill is delivered via Billogram (e.g. Bredband2,
  and other Swedish/EU billers) and you need the per-invoice OCR to pay it —
  the OCR changes on every invoice, so you MUST fetch it from the link rather
  than reuse an old one. Pairs with the pay-wise-bill skill.
---
# Billogram invoice OCR

1. Open the Billogram link (`billogram.com/l/<id>` or `/r/<id>`).
2. Read the OCR/reference number, bankgiro, amount, and due date off the invoice.
3. Hand them to `pay-wise-bill` to schedule the payment.
