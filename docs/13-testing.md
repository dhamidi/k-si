# 13 — Testing: three rings

käsi is tested in **three rings**, from innermost (fast, deterministic, run
constantly) to outermost (slow, real, run rarely). All three rings run the
*same scenario scripts* ([14](./14-test-language.md)) against the *same
application code*; the only thing that changes between rings is which
implementation of each edge is wired in ([12](./12-development-process.md),
*The seam rule*).

```
┌───────────────────────────────────────────────────────────────┐
│  Ring 3 — LIVE            real harness, real mail, real disk  │
│    rare, scheduled, spends money · every run records          │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │  Ring 2 — RECORDED    edges replay captured reality     │  │
│  │    deterministic, real bytes · run in CI                │  │
│  │  ┌───────────────────────────────────────────────────┐  │  │
│  │  │  Ring 1 — SIMULATION   everything in memory       │  │  │
│  │  │    milliseconds, run constantly, fleets of 100    │  │  │
│  │  └───────────────────────────────────────────────────┘  │  │
│  └─────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────┘
        recordings flow inward:  live → cassettes → fixtures
```

Each ring answers a different question:

| Ring | Question it answers | Determinism | When it runs |
|------|--------------------|-------------|--------------|
| 1 — Simulation | Is the **logic** right? | Total | Every edit, every commit |
| 2 — Recorded | Does the logic handle **reality's actual bytes**? | Total | Every commit / CI |
| 3 — Live | Does the system work against the **real world**? | None | Scheduled; on edge changes; pre-release |

## Ring 1 — Simulation

An **instance** in the simulation ring is the käsi application assembled from
the same modules `main.go` assembles ([01](./01-architecture.md)) — every
handler, effect, and subscription — but wired to simulated edges: an
in-memory mailbox, a scripted harness, a virtual clock, in-memory SQLite, a
throwaway workspace tree, sentinel-valued secrets
([12](./12-development-process.md)). Nothing is stubbed *inside* the seam; an
instance routes mail, creates tasks, "runs" agents, assembles MIME replies,
archives, and replays its log exactly as production does. A script may also
assemble a **partial** application (`use email tasks`) when it wants to drive
a few domains' handlers directly ([14](./14-test-language.md)).

Because an instance is just goroutines and RAM, they are cheap:

- **One instance** boots in microseconds. A scenario drives it through a whole
  multi-turn task — delivery, agent turns, replies, completion, archival — in
  a few milliseconds.
- **A fleet** is many instances in one test process — `kasi test -n 100`
  runs a hundred copies of a script concurrently — deliberately beyond the
  scale envelope of ~100 concurrent agent runs
  ([12](./12-development-process.md)) — proving headroom on every run, and
  incidentally proving instances share no hidden global state (a package-level
  variable shows up as fleet cross-talk immediately).

### Virtual time and `settle`

Two rules make simulation deterministic:

- **Time is virtual.** The clock edge only moves when the scenario says
  `clock advance 5m`. Timers, polling intervals, and timeout logic all key off
  messages carrying virtual time, per the determinism principle of
  [01](./01-architecture.md). No test ever sleeps.
- **Synchronisation is quiescence, not delay.** The `settle` command runs an
  instance until it is quiescent: the inbound channel is empty, no effect
  worker is mid-flight, and no virtual timer is due. After `settle`, the model
  is in a stable state and assertions are race-free. There is no "wait 100ms
  and hope" anywhere in the suite.

### Fault injection

The simulated edges accept scripted misbehaviour, which is how the durability
design ([01](./01-architecture.md), [03](./03-persistence.md)) is continuously
verified rather than merely believed:

- **Crash and restart.** `crash` discards an instance's model and live
  goroutines, keeping only what production would keep (the log and content
  tables). `restart` replays the full log with effects suppressed and starts
  subscriptions. Scenarios crash instances at adversarial moments — between
  the outbox write and `mark-email-sent`, mid-agent-run — and assert that
  replay plus reconciliation land in the right state with no duplicated
  effects. This is Flow E of [10](./10-flows.md) as a repeatable test, at any
  point in any scenario.
- **Failing effects.** Any simulated edge can be told to fail its next N
  operations (mail send errors, harness spawn failures). Scenarios assert that
  the outbox stays `pending`, reconciliation retries, and exactly-once
  delivery holds.
- **Stopped runs.** The scripted harness can hang until signalled, exercising
  the stop path ([05](./05-agents-and-tasks.md)) deterministically.

### Standing invariant checks

Some properties are checked automatically by the scenario runner, in every
scenario, rather than being individual tests — so they can never be forgotten:

- **Replay convergence.** After a scenario finishes, the runner rebuilds the
  model by folding the instance's log from zero and asserts it equals the live
  model. Every scenario is thereby also a replay test.
- **No plaintext secrets.** The simulated secrets edge hands out sentinel
  values. The runner scans the message log, the model, and all durable tables
  for those sentinels; any hit fails the scenario. This mechanically enforces
  the references-until-the-edge rule ([06](./06-secrets.md)).
- **Handlers do no I/O.** Simulated edges record the goroutine that calls
  them; a call from the reducer goroutine fails the scenario. The purity rule
  of [01](./01-architecture.md) is enforced, not just documented.
- **Archive before delete.** The simulated workspace refuses deletion while
  any file in it lacks a matching archive row, enforcing the invariant of
  [05](./05-agents-and-tasks.md).

## Ring 2 — Recorded

The simulation ring proves the logic; it cannot prove that käsi understands
what the real world actually sends. Ring 2 closes that gap deterministically:
edges replay **cassettes** — captures from real ring-3 runs — so the bytes are
real but the test is exact and offline.

Kinds of recordings, each captured at its edge's interface:

| Recording | Captured from | Replayed by |
|-----------|--------------|-------------|
| **Harness run** — the inputs laid into `in/`, the session transcript (verbatim JSONL), the files left in `out/`, the exit status, per turn | A live agent run ([05](./05-agents-and-tasks.md)) | The recorded-harness edge: it emits the captured transcript, writes the captured `out/`, exits with the captured status |
| **Mail exchange** — provider calls and responses at the mail-provider interface | A live JMAP session ([04](./04-email.md)) | The recorded mail edge |
| **Raw MIME** — full RFC 5322 bytes of real inbound mail (real clients, real forwards, real oddities) | The live inbox | `deliver -raw` in any scenario; the best parser test cases are the ones reality wrote |
| **Message log** — the complete log of a live or recorded run | Any instance ([03](./03-persistence.md)) | Replayed against the current build: it must fold without error and reach a coherent model. Unknown tags must drop, not crash — this is the open-set compatibility promise of [01](./01-architecture.md), tested against genuinely old logs |

Cassettes live under `t/cassettes/`, named by scenario. They are committed:
the deterministic suite must run with no network and no credentials.

**Staleness is explicit, never silent.** When replaying a harness cassette,
the recorded edge compares the inputs the system lays into `in/` against what
was recorded. A mismatch fails with "cassette stale — re-record via the live
ring", pointing at the probe that refreshes it. A cassette is a claim about
reality; when the system changes what it says to reality, the claim must be
re-earned, not fudged.

Recorded scenarios assert **outcomes, not wording** — a real agent's reply
text varies between recordings, so assertions match on structure (a reply was
sent, to these participants, with this attachment) and use pattern matching
for content ([14](./14-test-language.md)).

## Ring 3 — Live

Rare but important: the outermost ring runs against the actual world.

What it exercises — precisely the things no simulation can prove:

- **Real harness runs.** Actually spawning each configured harness adapter,
  resuming a real session across turns, stopping a real process, locating and
  capturing a real transcript. Every adapter must pass the **harness
  conformance suite** here ([12](./12-development-process.md)) — this, not any
  vendor specifics, is what keeps käsi honestly harness-agnostic.
- **Real mail round-trips** through the real provider on a dedicated test
  domain and account: catch-all delivery, threading across real
  `Message-ID`s, submission, attachment upload/download.
- **Real provisioning**: mise actually installing pinned tools into a real
  workspace ([07](./07-skills-and-tools.md)).

Ground rules:

- **Scheduled, not gating.** Live probes run on a schedule and on demand —
  when an edge or adapter changes, and before a release. They are never in the
  inner loop and never a prerequisite for merging logic changes; rings 1–2
  gate those.
- **Isolated and cheap.** Probes use a test mail domain, a scratch workspace
  root, and minimal prompts. They spend real money on real agents; the suite
  is sized accordingly — a handful of probes that each earn their keep.
- **Every live run records.** Recording is not a special mode; the live edges
  always capture. A green probe refreshes cassettes; a red probe leaves a
  complete recording — transcript, exchanges, message log — as the debugging
  artifact.

## The graduation loop

The rings are connected by a deliberate flow of information inward:

```
 live probe runs (ring 3)
      │  captures: transcripts, out/, exchanges, raw MIME, logs
      ▼
 cassettes & fixtures (t/cassettes/, t/fixtures/)
      │  replayed deterministically (ring 2), forever, offline
      ▼
 distilled cases feed simulation scenarios (ring 1)
```

An expensive, flaky fact about the real world is paid for **once**, in ring 3,
and then held as a deterministic regression test from then on. When reality
changes — a provider alters a response shape, a harness changes its transcript
format — a live probe fails or a cassette goes stale, the recording is
refreshed, and the delta is reviewed like any other diff: the cassette diff
*is* the changelog of the outside world.

## Determinism rules

The full list of what keeps rings 1 and 2 exactly reproducible — most are
restatements of the architecture's own principles ([01](./01-architecture.md)),
which is not a coincidence:

1. **Virtual time.** The clock is an edge; scenarios advance it explicitly.
2. **No sleeps.** Synchronisation is `settle` (quiescence), never delay.
3. **Deterministic ids.** Ids derive from log offsets; anything genuinely
   random is generated at an edge and enters as a recorded message value.
4. **One channel, one reducer.** Message application is totally ordered;
   parallelism lives only in effects, whose results re-enter as ordered
   messages.
5. **Recorded reality.** Ring 2 edges emit exactly their cassettes; staleness
   is a loud failure, not a quiet re-fetch.

If a scenario is flaky, one of these rules is being broken — flakiness is
always a bug in an edge or the runner, never something to retry away.
