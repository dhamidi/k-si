# Removing a skill

*take a skill out of the collection, from the web UI*

You author skills so agents can follow a procedure you've taught them once — a way
to reconcile an invoice, a script to pull a report. Sometimes a skill outlives its
use, or an agent authored one that turned out wrong. Removing a skill takes it out
of the collection, so käsi stops handing it to future tasks.

## What removal does

Open `/skills`. Every skill you or an agent has authored is listed, newest first,
each with its name, a one-line description, and where it came from — an agent that
authored it during a task, or you through the UI. Next to each skill is a **Remove**
button, and the same button sits at the top of a skill's own page.

Press Remove and the skill is gone in one step: it drops off the list, and the next
task käsi runs no longer finds it in its workspace. There's no confirmation and no
second screen — one press removes it, the way you'd delete a memory you no longer
want.

## Where a skill lives, and why removal touches two places

A skill is kept in two parts, and Remove clears both:

- **Its body** — the whole skill directory, scripts and assets and all — is stored
  as one archive off to the side, because it can be large. This is the copy käsi
  lays into every task's workspace so the agent can run it. Removing a skill deletes
  this archive, and that's what actually stops the skill from reaching future tasks.
- **Its entry in the registry** — the short listing (name, description, origin) you
  see on `/skills`. This lives in käsi's log, so removing it is recorded as an event,
  and the registry rebuilds itself correctly every time käsi replays its history.

You don't have to think about the split. One press of Remove clears both. It matters
only in that removal is thorough: the skill is gone from the listing *and* from the
next task's workspace, not just hidden from view.

## What removal keeps, and what it doesn't

The registry is a log, so a removal is recorded, not erased: you can still see that
a skill was in the collection and that it was taken out. The skill's body is
different. The archive is deleted outright — that's the whole point, since it's what
would otherwise keep reaching tasks — so don't treat Remove as "hide for now." If
you want a removed skill back, an agent authors it again.

## When a removed skill comes back

Removal takes a skill out of the collection as it stands. If an agent later authors
a skill with the same name — during a task, writing it to its outbox — that's a new
authoring, and the skill returns to the registry with fresh contents. Removal clears
what's there now; it isn't a permanent ban on the name.

## Removing a skill vs. forgetting a memory

Removing a skill is the sibling of forgetting a memory. Both are plain, one-press
acts of curation you do from the web UI, and both take a named unit out of what
future tasks are handed. The difference is what sits underneath. A memory is a small
fact stored as a single log entry, so forgetting it is one record. A skill's body is
a larger archive kept alongside the registry — so removing a skill clears that
archive as well as the log entry.

## Limitations

- **The whole skill, or nothing.** Remove takes out the entire skill. There's no way
  to remove one file inside it, or to pause a skill without taking it out.
- **Same-name re-authoring returns it.** Removal clears the current collection; an
  agent that later writes a skill under the same name brings it back. Removal isn't a
  reservation on the name.
- **One collection.** There's a single shared registry. Removing a skill removes it
  for every task, not for one kind of task.
