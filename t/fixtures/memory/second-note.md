---
name: second-note
description: the entry listed immediately after the multiline one — it must stay a proper list item
type: reference
---
This memory exists only to sit directly below `multiline-desc` in the index, so a
scenario can prove that folding the block-scalar description to one line leaves this
following `- [...]` entry intact rather than shredded by leaked newlines.
