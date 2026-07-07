# Memory

*durable facts, carried from one task to the next*

käsi works one task at a time — a different email thread, a different agent
session, sometimes weeks apart. Memory is what it carries across them: a set of
durable facts about you and your world, laid in front of every worker agent
before it starts.

A memory is a small note in a file — one fact, preference, or pointer. käsi keeps
a collection of them, copies the whole thing into each agent's workspace as
ordinary files, and records whatever the agent adds or removes so the next task
sees the update. Files are the interface; the bookkeeping is käsi's.

Status: this is the design of record. The implementation is being built to it.

## What a memory is

A single Markdown file with a short YAML header:

```markdown
---
name: reply-style
description: the owner prefers terse, plain-text replies without preamble
type: feedback
---
Keep replies short and direct. No "I hope this finds you well", no restating the
question, no summary of what you're about to do. Lead with the answer.
```

- `name` — a short slug, and the file's name (`reply-style.md`). It's the memory's
  identity: writing the same name again *updates* that memory instead of adding a
  second one.
- `description` — one line saying what the memory holds and when it matters. This
  is what shows up in the index, so the agent can tell at a glance whether to open
  it.
- `type` *(optional)* — one of `user`, `feedback`, `project`, `reference`. A hint
  for grouping and nothing more; käsi doesn't treat the categories specially.

Everything below the second `---` is the note itself, as long or short as the fact
needs.

The frontmatter reader is the same one that parses a skill's `SKILL.md`. That's
deliberate: a skill is a reusable *procedure*, a memory is a durable *fact*. Both
are named, both carry a one-line description, both get provisioned into every run,
so they share machinery where it makes sense.

## How memory reaches a task

Before a worker starts, käsi lays the collection into the task's input directory:

```
in/
├── body.txt              the message this task is about
├── invoice.pdf           any attachments
├── MEMORY.md             ← an index of every memory
└── memory/
    ├── owner-timezone.md  ← the memories themselves, one file each
    ├── wise-account.md
    ├── reply-style.md
    └── news-sources.md
```

`in/memory/` holds the notes, one file each. `in/MEMORY.md` is an index käsi
generates from them — one line per memory, name linked to its file, description
after:

```markdown
# Memory

Durable facts käsi has learned. Each links to a note in ./memory/.

- [owner-timezone](memory/owner-timezone.md) — the owner is in Europe/Berlin; schedule and phrase times accordingly
- [wise-account](memory/wise-account.md) — the owner's Wise account id and how to reach it through the CLI
- [reply-style](memory/reply-style.md) — the owner prefers terse, plain-text replies without preamble
- [news-sources](memory/news-sources.md) — the sites to consult for the daily brief
```

The agent reads `MEMORY.md` first and opens the notes that look relevant. For a
handful it'll just read them all; as the collection grows, the index is what keeps
memory useful instead of overwhelming — the agent spends attention only on what
bears on the task in front of it. Same progressive-disclosure bet käsi makes with
skills: keep the always-loaded surface to a name and a line, pull the full text on
demand.

käsi generates the index; the agent never writes it. The agent only authors
individual notes, and käsi keeps `MEMORY.md` in step, so it can't drift out of
sync with what it lists. Both `MEMORY.md` and `memory/` are rebuilt fresh every
run — which is why editing a file in `in/memory/` in place does nothing lasting
(see the rules below).

## Remembering

To remember something, the agent writes a note to its outbox, `out/memory/`, in
the same shape:

```markdown
---
name: wise-recipients
description: the owner's saved Wise recipients, cached in ./store/wise.db
type: reference
---
Pulled with `wise-cli` and cached in ./store/wise.db (table `recipients`). Read
from the cache before calling the API again; refresh only when a name is missing.
```

When the task finishes, käsi harvests `out/memory/` and the note joins the
collection. The next task finds `wise-recipients.md` in `in/memory/` and its line
in the index.

Updating is the same act with an existing name: write `out/memory/reply-style.md`
with new content, and because the name already exists, käsi replaces it in place
instead of adding a duplicate.

## Forgetting

To forget a memory, the agent deletes its file from `in/memory/`.

That's the whole gesture. The agent was handed the collection; to drop a note, it
removes the copy it was given. Deletion is the only way to forget, and it's done
in `in/` rather than the outbox on purpose: throwing a file away is the plainest
possible "I don't want this," and it needs no special marker.

## The rules of the working set

`in/memory/` isn't a read-only display — it's the agent's working set, a copy it
may prune. Four rules keep it predictable:

- **Deletion forgets.** Remove `in/memory/<name>.md` to forget that memory.
- **The outbox remembers.** Write `out/memory/<name>.md` to add or update one.
- **In-place edits are ignored.** Editing a file inside `in/memory/` without going
  through `out/` changes nothing — it's a scratch copy, overwritten from the log
  next run. Change content through the outbox; use `in/` deletion only to remove.
- **The outbox wins ties.** If the same task both deletes `in/memory/<name>.md` and
  writes `out/memory/<name>.md`, the write wins — käsi reads that as a change of
  mind.

Honoring only deletion, not arbitrary edits, from `in/` keeps the signal clean: a
missing file means one thing, an in-place edit could mean anything.

## Who changes memory

Two parties write memory, through the same record:

- **The agent, during a task** — the everyday path above. Reads `in/memory/`,
  remembers via `out/memory/`, forgets by deletion.
- **You, through the web UI** — a `/memory` page lists every memory with its
  description, lets you read the full text, and lets you edit, add, or delete
  directly. This is how you curate: prune a fact the agent got wrong, sharpen a
  description, or seed a memory by hand before any task has learned it.

Both routes end in the same place, so a fact you wrote and a fact the agent learned
are indistinguishable afterward.

## How it works

The collection isn't a folder tasks share. It lives in käsi's event log as a
sequence of directives — `remember` and `forget` — and the current set is what you
get by replaying them. The files an agent sees are a projection of that log, rebuilt
per run; the files it leaves behind are translated back into new directives. The
agent never touches the log; the log never holds a file. käsi is the translator,
and the same translation serves the web edits.

**Remember** harvests `out/memory/foo.md` into a `remember{name, content}`
directive, where `content` is the raw file, frontmatter and all. The description
isn't stored — it's *derived* from the content by the reducer, on every replay.
Same principle as the skills registry: store the raw fact, derive everything else
in the pure reducer. Change how a description (or the index) is parsed and every
memory corrects itself on the next replay, no migration. `remember` is an upsert
keyed by name: a new name appends, an existing name replaces and bumps the version.
Because the key is the name, two tasks that remember *different* things never
collide, even running close together — the payoff over one shared `MEMORY.md`,
where they'd clobber each other.

**Forget** is a diff. käsi can't tell "the agent deleted this" from "this was never
here" by looking at `in/memory/` alone, so at the start of a run it records exactly
which memories it provisioned into *this* run. When the task ends it compares that
against the survivors:

```
forgotten = provisioned − survivors
```

and emits `forget{name}` for each that vanished. The diff is against *this run's*
provisioned set, not the collection as it stands now — load-bearing, because a
memory some other task added while this one ran has no file here and would otherwise
look deleted.

**Ties** are directive precedence: a `remember` for a name suppresses a `forget`
for the same name in the same harvest.

The full copy handed to each run is a transient input, not a log entry, so
provisioning doesn't grow the log — only real `remember` and `forget` directives do.

## Why a log, not a folder

Memory could have been a shared folder, or a row updated in place. It's an event
log because the facts käsi keeps are small, few, and precious — exactly where a log
earns its keep:

- **History.** Every `remember` and `forget` is dated and kept. You can see what
  käsi learned, when, and from which task.
- **Recovery.** A bad edit isn't destructive — the previous content is still in the
  log, and an earlier state can be rebuilt. Nothing is ever truly overwritten.
- **Replay.** käsi rebuilds all its state by replaying the log. Memory rebuilds with
  everything else, no separate store to keep consistent.

Because a memory is short text, the cost that rules this out for bulky data —
writing every version into the log — is negligible.

## Memory and skills

Memory is the sibling of skills. A skill is a procedure the agent can follow; a
memory is a fact it should know. Built alike — named units, a one-line description,
harvested from output, provisioned into every run — but different where it counts.
A skill is stored as an opaque archive in a side table, because it can be large
(scripts, assets). A memory is a log directive, because it's small and its history
matters. A skill lands in `.claude/skills/`, for when the agent is *doing*; a memory
lands in `in/memory/`, for when it's *knowing*.

## Limitations

- **One collection for everything.** There's a single collection shared by every
  task. Scoping memory to particular kinds of task (a per-route collection) is a
  later refinement if it proves useful.
- **Same-name races.** Naming removes almost all conflict between concurrent tasks;
  what's left is two tasks updating the *same* memory at once, where the later write
  wins. Runs are largely sequential, so it's rare — an optimistic version check is
  the fix if it ever bites.
