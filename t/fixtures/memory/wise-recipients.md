---
name: wise-recipients
description: the owner's saved Wise recipients, cached in ./store/wise.db
type: reference
---
Pulled with `wise-cli` and cached in ./store/wise.db (table `recipients`). Read
from the cache before calling the API again; refresh only when a name is missing.

Known recipients (as of the last refresh):

- Bredband2 AB — SE bankgiro, for the monthly broadband invoice.
- Vattenfall — SE bankgiro, electricity.
- Anna Lindqvist — personal, IBAN SE35 5000 0000 0549 1000 0003.
