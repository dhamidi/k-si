# Browsing the store

*a window into the agent's private disk*

Every agent run gets a directory at `./store/` that survives the task. The rest
of the workspace is wiped the moment a task finishes; the store persists. It's
where the agent keeps SQLite databases, caches, downloaded files, and scratch
scripts, shared across all its tasks, so it can read from it before re-fetching
anything it cached before.

The store lives outside käsi's event log. Everything else käsi does — every email,
every run, every remembered fact — is a directive in the log, and you can
reconstruct all of it by replay. The store isn't: it's a real directory on disk
that the agent writes to directly, and replay never rebuilds it. So the store is
the one part of käsi's state the log can't show you.

This page is the window into it. `/store` lets you walk that directory, read what's
in it, and pull files out — so you can see what the agent has squirreled away,
check whether a cache is stale, or debug why it didn't re-fetch.

Status: this is the design of record. The store itself is built (Flow F,
decision-012); the browse page is not yet.

## Store vs. memory

The agent's system prompt calls the store its "durable, private memory," which
collides with käsi's actual *Memory* feature. They're different things.

**Memory** is a set of small, named facts — one Markdown note each — that live in
the event log. You curate them on the `/memory` page: edit, sharpen, delete. Every
change is a log event.

**The store** is a live directory of files the agent writes directly — databases,
caches, whatever it needs to remember between tasks. It's not in the log, not
curated, and not uniform. Browsing memory is *curation*; browsing the store is
*inspection*. You look to understand what the agent kept, not to shape it.

## What you'll see

The store is a plain directory tree, so the page is a plain file browser:

```
/store
├── apps/                        registered web apps live here (see Apps)
│   └── accounting/              one app's code and its own data
├── wise.db              412 KB   SQLite — the owner's cached Wise recipients
├── news-cache/
│   ├── 2026-07-06.json   18 KB
│   └── 2026-07-07.json   19 KB
└── scrape.py            1.2 KB   a scratch script the agent wrote
```

An app's directory under `apps/` holds its code and its own data (the accounting
app's ledger, say), so browsing the store is also how you peek at what an app has
saved. `/store` lists the root. Each entry shows its name, size, and when it last changed;
directories link deeper, files link to a view. Nested paths follow the same
convention as skills — `/store/news-cache/2026-07-07.json` is a URL you navigate to,
not a thing you assemble by hand.

A text file renders inline, the way a skill file does. A SQLite database or any
other binary doesn't — rendering a database as text is noise — so instead you get
its details and a download link. Every file downloads, text or not.

## It's live, not replayed

Because the store isn't in the log, the page reads the directory straight off disk
on every request. There's nothing to project and nothing to fold — you see exactly
what's on disk right now, including a file a running agent wrote a second ago.

This is the same way käsi already serves a running task's transcript and a task's
archived artifacts: read the edge, don't replay the log. The contrast is the
`/memory` page, where the descriptions you see are derived by replaying the log.
The store has no such projection — it's just the directory.

## Read-only

You can browse and download; you can't edit. The store is the agent's live working
data, and an agent may be writing it at the very moment you're looking — SQLite
handles many readers and one writer, and käsi doesn't serialize access. Editing a
file out from under a running agent would corrupt exactly the cache the feature
exists to let you inspect.

So to change what the agent knows, you don't reach into the store. You curate a
*memory*, or you let the agent re-cache on its next run. The store is something you
watch, not something you hand-edit.

## How it works

Reads go through the store's filesystem edge — the same edge that symlinks
`./store/` into every run. The web server opens the directory read-only and lists
or streams whatever you asked for, live. No log, no model, no projection sits in
between; there's nothing to derive because the store was never model state.

Like the rest of käsi's browse pages, `/store` is host-gated and carries no
separate login — reaching it means you already have access to the machine käsi runs
on.

## Limitations

- **No pruning yet.** The store has no quota and nothing ever garbage-collects it,
  so it only grows. Browsing is read-only today; a delete action is the obvious next
  step, but it has to be careful — the file you'd remove may be one a running agent
  is mid-write on, and unlike a forgotten memory, a deleted store file isn't in the
  log to replay back. It's gone.
- **Big files download, they don't preview.** A database or a binary isn't rendered
  inline; you pull it down to open it locally. Text previews inline up to a modest
  size, then it's a download too.
- **A live database may download torn.** Grab a SQLite file while the agent is
  writing it and you may get a snapshot mid-transaction, without its `-wal`
  sidecar — fine for a glance, not a backup. Read it when no task is running if you
  need it clean.
- **One store, one tree.** There's a single store shared across the whole assistant,
  so the page shows one tree. Per-route stores (a separate `./store/` per kind of
  task) are a later refinement if they prove useful.
