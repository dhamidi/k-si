# Memory

käsi remembers. Between one task and the next — different email threads,
different agent sessions, sometimes weeks apart — it carries a set of durable
facts it has learned about you and your world, and lays them in front of every
worker agent before it starts. That set is käsi's *memory*.

This manual explains what a memory is, how it appears inside a task, how the
agent (and you) add, change, and remove memories, and — in the asides marked
*Under the hood* — how the whole thing works and why it is built that way.

> **Status (for developers).** This document specifies the memory feature. It is
> the design of record and is under construction; where the manual says "käsi
> does X," read that as the intended behaviour the implementation is being built
> to satisfy.

*In this chapter:* [The idea in one minute](#the-idea-in-one-minute) ·
[What a memory is](#what-a-memory-is) ·
[How memory reaches a task](#how-memory-reaches-a-task) ·
[Remembering and updating](#remembering-and-updating) ·
[Forgetting](#forgetting) ·
[The rules of the working set](#the-rules-of-the-working-set) ·
[Who changes memory](#who-changes-memory) ·
[A worked example](#a-worked-example) ·
[Design notes](#design-notes)

---

## The idea in one minute

A memory is a small note — a single fact, preference, or pointer — written in a
file. käsi keeps a collection of them.

In every task, käsi copies the whole collection into the agent's workspace, where
the agent reads it as ordinary files. When the agent learns something worth
keeping, it writes a new note into its outbox; when a note stops being true, it
deletes the copy it was given. käsi watches what changed and records it, so the
next task — and every task after — sees the updated collection.

That is the entire model. **Files are the interface; everything else is
bookkeeping käsi does on your behalf.**

> **Under the hood.** The collection is not a folder somewhere that tasks share.
> It lives in käsi's event log as a sequence of *directives* — `remember` and
> `forget` — and the current set of memories is what you get by replaying them.
> The files an agent sees are a *projection* of that log, materialised fresh for
> each run; the files an agent leaves behind are *translated back* into new
> directives. See [Design notes](#design-notes) for why the log, and not a
> folder, is the source of truth.

---

## What a memory is

A memory is a single Markdown file with a short YAML frontmatter header:

```markdown
---
name: reply-style
description: the owner prefers terse, plain-text replies without preamble
type: feedback
---
Keep replies short and direct. No "I hope this finds you well", no restating the
question, no summary of what you're about to do. Lead with the answer.
```

Three things describe every memory:

  * **`name`** — a short slug that is also the file's name (`reply-style.md`).
    The name is the memory's identity: writing to the same name later *updates*
    that memory rather than adding a second one.

  * **`description`** — a single line that says what the memory holds and when it
    matters. This line is what appears in the index (below), so the agent can
    tell at a glance whether a memory is worth opening.

  * **`type`** *(optional)* — a coarse category, one of `user`, `feedback`,
    `project`, or `reference`. It is only a hint for grouping and curation; käsi
    does not treat the categories specially.

Everything below the second `---` is the memory itself, plain Markdown, as long
or as short as the fact requires.

> **Under the hood.** The frontmatter reader is the same one that parses a
> skill's `SKILL.md` header (`name` + `description`). A memory is deliberately
> the small cousin of a skill: a skill is a reusable *procedure* (how to do
> something), a memory is a durable *fact* (something that is true). Both are
> named, both carry a one-line description, both are provisioned into every run —
> so they share machinery wherever it makes sense.

---

## How memory reaches a task

Before a worker agent starts, käsi lays the memory collection into the task's
input directory, `in/`:

```
in/
├── body.txt                     the message this task is about
├── invoice.pdf                  any attachments
├── MEMORY.md                    ← an index of every memory
└── memory/
    ├── owner-timezone.md        ← the memories themselves, one file each
    ├── wise-account.md
    ├── reply-style.md
    └── news-sources.md
```

`in/memory/` holds the memories, one file per note. `in/MEMORY.md` is an
**index** käsi generates from them — one line per memory, its name linked to its
file, followed by its description:

```markdown
# Memory

Durable facts käsi has learned. Each links to a note in ./memory/.

- [owner-timezone](memory/owner-timezone.md) — the owner is in Europe/Berlin; schedule and phrase times accordingly
- [wise-account](memory/wise-account.md) — the owner's Wise account id and how to reach it through the CLI
- [reply-style](memory/reply-style.md) — the owner prefers terse, plain-text replies without preamble
- [news-sources](memory/news-sources.md) — the sites to consult for the daily brief
```

The agent is told, in its standing instructions, to read `in/MEMORY.md` first and
open the individual notes that look relevant. For a handful of memories it will
simply read them all; as the collection grows, the index is what keeps memory
useful instead of overwhelming — the agent spends attention only on the notes
that bear on the task in front of it.

> **Under the hood.** This is *progressive disclosure*, the same bet käsi already
> makes with skills: keep the always-loaded surface small (a name and a
> description) and let the agent pull the full text on demand. The index is
> **generated by käsi, never written by the agent** — the agent only ever authors
> individual notes, and käsi keeps `MEMORY.md` in step with them, so the index
> can never drift out of sync with what it lists.
>
> `in/MEMORY.md` and `in/memory/` are a read projection of the log, rebuilt on
> every run. That is why editing a file in `in/memory/` in place has no lasting
> effect (see [The rules of the working set](#the-rules-of-the-working-set)): the
> next task overwrites it from the log.

---

## Remembering and updating

To remember something new, the agent writes a note into its **outbox**,
`out/memory/`, using the same file shape:

```
out/
└── memory/
    └── wise-recipients.md
```

```markdown
---
name: wise-recipients
description: the owner's saved Wise recipients, cached in ./store/wise.db
type: reference
---
Pulled with `wise-cli` and cached in ./store/wise.db (table `recipients`). Read
from the cache before calling the API again; refresh only when a name is missing.
```

When the task finishes, käsi harvests `out/memory/`, and the note becomes part of
the collection. The next task will find `wise-recipients.md` waiting in
`in/memory/`, and its line in the index.

**Updating a memory is the same act with an existing name.** To correct or extend
`reply-style`, the agent writes `out/memory/reply-style.md` with the new content;
because the name already exists, käsi replaces that memory in place rather than
adding a duplicate.

> **Under the hood.** Harvesting `out/memory/foo.md` emits a `remember` directive
> — `remember{name, content}` — into the log, where `content` is the raw memory
> file, frontmatter and all. The description is **not** carried: it is *derived*
> from the raw content by the reducer, on every replay. This is the same principle
> the skills registry follows — store the raw fact in the log, derive everything
> else in the pure reducer that runs on replay — so a change to how a description
> (or the `MEMORY.md` index) is parsed corrects every memory on the next replay,
> with no migration. `remember` is an upsert keyed by `name`: a new name appends a
> memory, an existing name replaces that one's raw content and bumps its version.
> Because the key is the name, two tasks that remember *different* things never
> collide, even if they run close together — each directive stands on its own.
> (This is the payoff over a single shared `MEMORY.md` document, where two tasks
> editing the same file would clobber one another.)

---

## Forgetting

To forget a memory, the agent **deletes its file from `in/memory/`.**

That is the whole gesture. The agent was handed the collection; to drop a note
from it, the agent removes the copy it was given. If `reply-style.md` no longer
reflects how you want replies written, the agent deletes `in/memory/reply-style.md`
during the task, and käsi records the removal when the task finishes.

Deletion is deliberately the *only* way to forget, and it is deliberately done in
`in/` rather than through the outbox: throwing a file away is the plainest
possible "I don't want this any more," and it needs no special marker or
convention to express.

> **Under the hood.** käsi cannot tell "the agent deleted this" from "this was
> never here" by looking at `in/memory/` alone — so at the start of a run it
> records exactly which memories it provisioned into *this* run's `in/memory/`.
> When the task finishes it compares that record against the files that survived:
>
> > `forgotten = provisioned − survivors`
>
> and emits a `forget{name}` directive for each one that vanished.
>
> The comparison is against **this run's provisioned set**, not the collection as
> it stands now — and that distinction is load-bearing. If käsi diffed against the
> live collection, a memory that some *other* task added while this one was
> running would have no file in this run's `in/memory/` and would look deleted.
> Pinning the diff to what this run was actually given closes that hole.

---

## The rules of the working set

`in/memory/` is not a read-only display; it is the agent's *working set* — a copy
it may prune. A few rules make it predictable:

  * **Deletion forgets.** Removing `in/memory/<name>.md` forgets that memory.

  * **The outbox remembers.** Writing `out/memory/<name>.md` adds or updates a
    memory.

  * **In-place edits are ignored.** Editing a file inside `in/memory/` without
    going through `out/` changes nothing: it is a scratch copy, overwritten from
    the log next time. Change a memory's *content* through the outbox; use `in/`
    deletion only to remove.

  * **The outbox wins ties.** If, in the same task, a memory is both deleted from
    `in/memory/` and written to `out/memory/`, the write wins — käsi reads that as
    the agent changing its mind and keeps the memory with the new content.

> **Under the hood.** The two gestures map onto the two directives — an `out/`
> write is a `remember`, an `in/` deletion is a `forget` — and the tie-break is
> just directive precedence: a `remember` for a name suppresses a `forget` for the
> same name in the same harvest. Honouring only deletion (not arbitrary edits)
> from `in/` keeps the signal unambiguous: a missing file means one thing, whereas
> an in-place edit could mean anything from a real change to an accidental
> scribble.

---

## Who changes memory

Two parties write to memory, through the same underlying record:

  * **The agent, during a task** — the everyday path described above. It reads
    `in/memory/`, remembers via `out/memory/`, forgets by deletion.

  * **You, the owner** — through käsi's web interface. A `/memory` page lists
    every memory with its description, lets you read the full text, and lets you
    edit, add, or delete notes directly. This is how you *curate*: prune a fact
    the agent picked up wrongly, sharpen a description, or seed a memory by hand
    before any task has learned it.

Both routes end in the same place, so a fact you write and a fact the agent
learned are indistinguishable afterwards — one collection, two ways to shape it.

> **Under the hood.** The `/memory` page emits the very same `remember` and
> `forget` directives the harvest does; the web form is simply another interface
> onto the log. The log is the single sink, and the agent's files, the index, and
> the web page are all faces on it. (This mirrors the rest of käsi: an inbound
> email and a form submission and a stopped run all become directives in the same
> log, applied by the same rules.)

---

## A worked example

*Monday.* You email käsi asking it to reconcile a Wise transfer. The worker agent
starts; `in/memory/` already holds `owner-timezone` and `reply-style` (you seeded
them). Reconciling, the agent pulls your recipient list with `wise-cli`, caches it
in `./store/wise.db`, and — so it needn't re-fetch next time — writes
`out/memory/wise-recipients.md`. It replies, terse and plain, the way
`reply-style` told it to. When the task ends, käsi records `remember{wise-recipients}`.

*Thursday.* A different email, a new task, a fresh workspace. `in/memory/` now
holds four notes, `wise-recipients` among them, and `in/MEMORY.md` indexes them.
The agent reads the index, sees it already knows your recipients, and reads them
from the note (and the cache) instead of calling Wise again.

*The following week.* You close your old Wise account. You open the `/memory`
page and delete `wise-recipients` yourself — it is stale and you would rather the
agent re-learn it cleanly. käsi records `forget{wise-recipients}`. The next task's
`in/memory/` no longer contains it, and the index no longer lists it. The fact is
gone from every future task, but every step that put it there and took it away is
still in the log.

---

## Design notes

This section collects the *why* behind the choices the asides introduced.

### Files in, directives out

The organising principle is a translation at two boundaries. On the way *in*, the
log's current set of memories is materialised into `in/memory/` and `in/MEMORY.md`.
On the way *out*, the agent's file gestures — writes to `out/memory/`, deletions
in `in/memory/` — are translated into `remember` and `forget` directives appended
to the log. The agent never touches the log; the log never holds a file. käsi is
the translator, and the same translation serves the owner's web edits.

This is not a special mechanism invented for memory. It is how käsi treats every
edge: inbound email becomes routing directives, a finished agent run becomes a
result directive, a click becomes a completion directive. Memory is one more edge,
translated the same way.

### Why the log, and not a shared folder

Memory could have been a folder on disk that every task reads and writes, or a
single row in a database updated in place. It is neither, because the facts käsi
keeps about you are small, few, and precious — exactly the case where an event log
earns its keep:

  * **History.** Every `remember` and `forget` is dated and kept, so you can see
    what käsi learned, when, and from which task.

  * **Recovery.** A bad edit is not destructive: the previous content is still in
    the log, and an earlier state can be reconstructed. Nothing about a note is
    ever truly overwritten.

  * **Replay.** käsi rebuilds its entire state by replaying its log from the
    beginning. Memory rebuilds with everything else, for free, with no separate
    store to keep consistent.

Because a memory is a short piece of text, the cost that would rule this out for
bulky data — writing every version into the log — is negligible here. (The full
copy of the collection that käsi hands each run is a transient input to that run,
not a log entry, so provisioning does not grow the log; only genuine `remember`
and `forget` directives do.)

### Memory and skills

Memory is the sibling of käsi's *skills*. A skill is a reusable procedure the
agent can follow; a memory is a durable fact the agent should know. They are
built alike — named units, a one-line description, harvested from the agent's
output, provisioned into every future run — but they differ where it counts.
A skill is stored as an opaque archive in a side table, because a skill can be
large (scripts, references, assets). A memory is stored as a log directive,
because it is small and its history matters. And a skill lands in
`.claude/skills/`, where the agent reaches for it when *doing*; a memory lands in
`in/memory/`, where the agent consults it when *knowing*.

### What is deliberately left for later

  * **One collection for everything.** Today there is a single memory collection,
    shared by every task, matching "käsi remembers, in every task." Scoping
    memory to particular kinds of task (a per-route collection) is a later
    refinement, if it proves useful.

  * **Same-name races.** Naming each memory removes almost all conflict between
    concurrent tasks; the residue is two tasks updating the *same* memory at once,
    where the later write wins. Runs are largely sequential, so this is rare; an
    optimistic version check is the fix if it ever bites.
