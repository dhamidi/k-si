---
name: owner-banking
description: >-
  The owner banks with Wise (multi-currency, holds EUR/USD/SEK) and Handelsbanken
  (a SEK current account); pay EU and cross-border invoices from the Wise balance
  and Swedish bankgiro/OCR bills from Handelsbanken, and always confirm the amount
  and the OCR with the requester before scheduling a transfer — never reuse an OCR
  across invoices.
type: reference
---
# Banking

- **Wise** — multi-currency, holds EUR/USD/SEK. Preferred for EU and
  cross-border payments; pay EU invoices from the EUR balance.
- **Handelsbanken** — SEK current account, for domestic bankgiro/OCR bills.

Confirm the amount and the OCR with the requester before every payment. The OCR
changes on every invoice, so fetch it fresh — never reuse an old one.
