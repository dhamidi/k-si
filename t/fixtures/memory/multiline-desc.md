---
name: multiline-desc
description: |
  First line of the description.
  Second line that must fold in.
  Third line.
type: reference
---
# A note with a literal block-scalar description

The frontmatter above uses a YAML literal block scalar (`|`), which preserves the
newlines between its lines. käsi must fold that description to a single physical
line before it reaches the one-line-per-entry MEMORY.md index, or the interior
newlines corrupt the index into stray, broken list items.
