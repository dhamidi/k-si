// feature-loop — the käsi feature-build loop, as far as it can be automated.
//
// Models the process spelled out algorithmically: recon → design+forks → decompose
// → (delegated worktree implement → adversarial verify). It captures ONLY the
// parts a subagent fan-out can own. The human/main-loop steps are deliberately
// NOT here — the workflow RETURNS them for you to drive:
//   - asking the forks (this returns them; you use AskUserQuestion)
//   - integrating cherry-picks, the state-leak + combined gate, commit(+trailers)+push
//   - writing the decision doc + the user feature doc to disk, and the memory write
//   - the live redeploy (operator-gated; never automated)
//
// It DRAFTS two docs for you (as strings, not files — the main loop writes them
// with the commit): the design-of-record (the decision-NNN "why", for maintainers)
// and a user feature doc (the "what it does / how you use it", for the operator).
//
// Two modes, one name:
//   Workflow({ name:'feature-loop', args:{ request:'<what to build>' } })
//       → understand + design; returns { map, design_of_record, feature_doc, forks, plan }. STOP.
//   Workflow({ name:'feature-loop', args:{ request:'…', answers:{…}, build:true } })
//       → runs one worktree implementer + adversarial verifier per DELEGATE chunk,
//         then re-writes the feature doc to match what shipped; returns
//         { delegated:[…], do_in_main_loop:[self/kit chunks], feature_doc, integration }.

export const meta = {
  name: 'feature-loop',
  description: 'The käsi feature-build loop (recon → design+forks → decompose → delegated worktree implement → adversarial verify). First pass returns the design-of-record + a user feature doc + genuine forks + plan; a build pass runs the delegatable chunks in isolated worktrees, verifies them, and re-writes the feature doc to match what shipped. Human-gated steps (fork Q&A, integration, gate/commit/push, writing the decision + feature docs, memory, live deploy) are returned, not performed.',
  whenToUse: 'A substantial käsi feature or change. Invoke once for the plan, ask the user the forks it returns, then invoke again with {answers, build:true} to fan out the implementers.',
  phases: [
    { title: 'Understand', detail: 'a scout lists the subsystems; parallel Explore agents deep-read each into a map' },
    { title: 'Design',     detail: 'draft the design-of-record and a user feature doc, surface the genuine forks, decompose into phases/chunks tagged self|kit|delegate' },
    { title: 'Implement',  detail: 'build:true only — one worktree-isolated implementer per delegate chunk, each gate-green AND copywriting-linter-clean on its own branch' },
    { title: 'Verify',     detail: 'build:true only — an adversarial reviewer per chunk (refute correctness/faithfulness) as each build lands' },
    { title: 'Polish',     detail: 'build:true only — a dedicated quality + copywriting edit pass over the whole changed surface (the gate does not check copy or taste); always runs' },
    { title: 'Document',   detail: 'build:true only — re-write the user feature doc to match what actually shipped, in the voice of docs/feature-*.md' },
  ],
}

const request = (args && args.request) || (typeof args === 'string' ? args : null)
if (!request) {
  log('feature-loop: pass { request } (then { request, answers, build:true }).')
  return { error: 'no request; args must be { request, answers?, build? }' }
}
const building = !!(args && args.build)
const answers = (args && args.answers) || {}

// ─────────────────────────── Phase 1: Understand ───────────────────────────
phase('Understand')

const AREAS_SCHEMA = {
  type: 'object',
  properties: {
    areas: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          name:  { type: 'string' },
          paths: { type: 'string', description: 'files/globs to read for this area' },
          why:   { type: 'string' },
        },
        required: ['name', 'paths'],
      },
    },
  },
  required: ['areas'],
}

const scout = await agent(
  `Scout the käsi repo for this request and list the 3–8 subsystems a builder must understand.
Request: ${request}
Return each area: name, the paths/globs to read, and why it matters. Read-only; do not edit.`,
  { label: 'scout', schema: AREAS_SCHEMA, agentType: 'Explore' },
)
const areas = (scout && scout.areas) || []
log(`${areas.length} area(s) to map`)

const MAP_SCHEMA = {
  type: 'object',
  properties: {
    area:      { type: 'string' },
    summary:   { type: 'string', description: 'what it does' },
    contracts: { type: 'string', description: 'key types/messages/commands/edges and their contracts' },
    seams:     { type: 'string', description: 'the exact files/functions a change would touch' },
    gotchas:   { type: 'string', description: 'invariants at play: replay convergence, twin fidelity, decision-004 secrets, no-URL/HTML lint, effects-suppressed-on-replay' },
  },
  required: ['area', 'summary', 'seams'],
}

const maps = (await parallel(areas.map((a) => () =>
  agent(
    `Deep-read this käsi area for the request "${request}".
Area: ${a.name}  (${a.paths})  ${a.why || ''}
Report what it does, its key contracts (types/messages/commands/edges), the seams a change would touch, and any invariant/gotcha in play. Read-only; do not edit.`,
    { label: `read:${a.name}`.slice(0, 48), phase: 'Understand', schema: MAP_SCHEMA, agentType: 'Explore' },
  ),
))).filter(Boolean)

// ──────────────────────────── Phase 2: Design ──────────────────────────────
phase('Design')

const DESIGN_SCHEMA = {
  type: 'object',
  properties: {
    design_of_record: {
      type: 'string',
      description: 'the "why" before code: problem, constraints, the decision(s), how it fits käsi (Elm-in-Go, event-sourced, effects-over-edges, twin rule, derive-on-replay, decision-004). What a decision-NNN doc would say.',
    },
    feature_doc: {
      type: 'string',
      description: 'the USER feature doc, as markdown — the "what it does / how you use it" guide, NOT the maintainer\'s "why". Same voice and shape as the existing docs/feature-*.md: a one-line tagline, what the feature is in plain terms, how to set it up and use it, and honest limitations. Second person, no internal jargon or decision-NNN/file references (copywriting skill). At design time this is the intended-behavior draft; the build pass rewrites it to match what shipped.',
    },
    forks: {
      type: 'array',
      description: 'ONLY genuine forks — each materially changes what gets built AND is the user\'s call. Recommend the first option.',
      items: {
        type: 'object',
        properties: {
          question:       { type: 'string' },
          why_it_matters: { type: 'string' },
          options:        { type: 'array', items: { type: 'string' } },
          recommendation: { type: 'string' },
        },
        required: ['question', 'options'],
      },
    },
    plan: {
      type: 'array',
      description: 'phases isolate risk (the riskiest — e.g. an edge→model migration — is its own phase). Each chunk tagged for dispatch.',
      items: {
        type: 'object',
        properties: {
          phase:    { type: 'string' },
          chunk:    { type: 'string' },
          dispatch: { type: 'string', enum: ['self', 'kit', 'delegate'], description: 'self = linchpin/subtle-invariant/irreversible; kit = deterministically scaffoldable via the kasi provider; delegate = bounded, mostly-independent worktree agent' },
          files:    { type: 'string' },
          tests:    { type: 'string' },
          notes:    { type: 'string' },
        },
        required: ['phase', 'chunk', 'dispatch'],
      },
    },
  },
  required: ['design_of_record', 'feature_doc', 'forks', 'plan'],
}

const design = await agent(
  `You are the käsi architect. From the subsystem map, produce (1) the design-of-record, (2) a user feature doc, (3) the genuine forks to ask the user, (4) a phase/chunk plan.
Request: ${request}
Map: ${JSON.stringify(maps)}
Rules:
- Design before code. Each phase must be independently buildable and gate-able; isolate the riskiest change as its own phase.
- Tag each chunk self|kit|delegate (see the schema). Prefer delegate for breadth, self for the linchpin and anything irreversible, kit where a manifest scaffolds it.
- Forks: only what changes the build AND is the user's to decide; recommend the first option; do NOT invent forks with obvious defaults.
- The feature_doc is for the OPERATOR, not the maintainer: what the feature does and how they use it, in plain second person. Read an existing docs/feature-*.md first and match its voice and shape (tagline, what-it-is, setup/use, limitations). No decision-NNN or file references, no internal jargon — the copywriting skill applies. This is the intended-behavior draft; the build pass will reconcile it with what actually ships.
- Honor the invariants: the log is the source of truth; twins mirror the real edge; effects are suppressed on replay; secrets are never rendered or logged; user-facing copy obeys the copywriting skill; the live service is never auto-deployed.`,
  { label: 'design', schema: DESIGN_SCHEMA },
)

if (!building) {
  log(`${design.forks.length} fork(s) to confirm · ${design.plan.length} chunk(s) planned`)
  return {
    map: maps,
    design_of_record: design.design_of_record,
    feature_doc: design.feature_doc, // user guide DRAFT (intended behavior) — the build pass reconciles it with what ships
    forks: design.forks,
    plan: design.plan,
    next: 'Ask the forks (AskUserQuestion), then re-invoke feature-loop with { request, answers, build:true }.',
  }
}

// ─────────────────── Phase 3+4: Implement + Verify (build) ──────────────────
phase('Implement')

const delegate = design.plan.filter((c) => c.dispatch === 'delegate')
const mine     = design.plan.filter((c) => c.dispatch !== 'delegate') // self + kit → main loop
log(`${delegate.length} chunk(s) to delegate · ${mine.length} for the main loop (self/kit)`)

const IMPL_SCHEMA = {
  type: 'object',
  properties: {
    branch:  { type: 'string' },
    summary: { type: 'string' },
    files:   { type: 'string' },
    gate:    { type: 'string', description: 'the final `mise run check` summary line' },
    notes:   { type: 'string', description: 'deviations, anything touched outside the stated files' },
  },
  required: ['summary'],
}
const VERDICT_SCHEMA = {
  type: 'object',
  properties: {
    ok:       { type: 'boolean', description: 'true only if you could NOT refute it' },
    findings: { type: 'array', items: { type: 'string' } },
  },
  required: ['ok'],
}

// pipeline: each chunk verifies as soon as its build lands — no barrier.
const built = await pipeline(
  delegate,
  (c) => agent(
    `Implement this käsi chunk in your ISOLATED worktree. Make \`mise run check\` GREEN, then commit to your branch (no push, no trailers).
Chunk: ${c.chunk}
Files: ${c.files || '(discover)'}    Tests: ${c.tests || '(add scenario coverage under t/)'}
Design: ${design.design_of_record}
Fork answers: ${JSON.stringify(answers)}
käsi rules: NO *_test.go (testlang scenarios only); pure handlers; effects over edges; copy-on-write; NO HTML-in-Go; reverse-route every URL; keep the sim twin faithful to the real edge.
Before finishing, do a copywriting pass on every user-facing string you added or changed (the gate does NOT check copy): follow the copywriting skill and run its linter until it reports 0 hard —
  bash ~/.agents/skills/copywriting/scripts/check-copy.sh <your changed files>
Report files, branch, the gate line, and the copy-linter result.`,
    { label: `impl:${c.chunk}`.slice(0, 48), phase: 'Implement', isolation: 'worktree', schema: IMPL_SCHEMA },
  ),
  (impl, c) => impl && agent(
    `Adversarially review this käsi change — try to REFUTE that it is correct, faithful to the design, and safe. Default ok=false if unsure.
Chunk: ${c.chunk}    Branch: ${impl.branch || '(worktree)'}    Built: ${impl.summary}
Check: correctness; replay convergence; twin fidelity (sim mirrors the real edge); decision-004 for any secret; the no-URL/no-HTML lint; whether the scenario coverage would actually catch a regression; and user-facing COPY quality (the gate does not check copy).`,
    { label: `verify:${c.chunk}`.slice(0, 48), phase: 'Verify', schema: VERDICT_SCHEMA },
  ).then((v) => ({ chunk: c.chunk, impl, verdict: v })),
)

const delegated = built.filter(Boolean)
const suspect = delegated.filter((d) => d.verdict && d.verdict.ok === false)

// ─────────────────────────── Phase 5: Polish ───────────────────────────────
// A dedicated quality + copywriting edit pass over the WHOLE changed surface —
// always runs, because the gate checks correctness, not copy or taste. Holistic
// on purpose: cross-chunk consistency (casing, units, terminology) is only
// visible looking at everything at once. Returns concrete before→after edits for
// the main loop to apply during integration (worktrees are per-agent, so this
// reviews the branches and hands back edits rather than committing them).
phase('Polish')
const POLISH_SCHEMA = {
  type: 'object',
  properties: {
    linter: { type: 'string', description: 'result of check-copy.sh across the changed files (aim: 0 hard, 0 soft)' },
    edits: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          file:   { type: 'string' },
          before: { type: 'string' },
          after:  { type: 'string' },
          why:    { type: 'string' },
        },
        required: ['file', 'before', 'after'],
      },
      description: 'concrete copy/quality edits: user-facing strings that read badly, leak internal jargon/doc refs, are ungrammatical, or are inconsistent across views',
    },
    overall: { type: 'string', description: 'a plain read-through verdict: does the changed surface read well to its actual reader?' },
  },
  required: ['edits', 'overall'],
}
const polish = await agent(
  `Do a dedicated QUALITY + COPYWRITING edit pass over everything this feature changed. The gate does NOT check copy — you are the only thing that catches prose and taste.
Request: ${request}
Changed by: ${JSON.stringify(delegated.map((d) => ({ chunk: d.chunk, branch: d.impl && d.impl.branch, files: d.impl && d.impl.files })))}
Also expect self/kit chunks the main loop will hand-write: ${JSON.stringify(mine.map((c) => c.chunk))}
Read every user-facing string in the changed files (component templates AND code copy: labels, buttons, help/description text, placeholders, empty states, validation/error messages, page titles, notification/email bodies, settings Short/Long). Apply the copywriting skill; run its linter:
  bash ~/.agents/skills/copywriting/scripts/check-copy.sh <changed files>
Return: the linter result, concrete before→after edits (plain language, grammar, consistency of casing/units/terminology across views, NO internal doc/decision/jargon leaks — technical identifiers may stay technical), and a read-through verdict. Read each string cold, as the reader.`,
  { label: 'polish', phase: 'Polish', schema: POLISH_SCHEMA },
)

// ─────────────────────────── Phase 6: Document ─────────────────────────────
// Re-write the user feature doc to match what SHIPPED, not what was designed —
// chunks deviate, forks were answered, scope shifts. Returned as markdown (not
// written to disk) for the main loop to save as docs/feature-<name>.md with the
// commit, exactly as the design-of-record becomes the decision doc.
phase('Document')
const DOC_SCHEMA = {
  type: 'object',
  properties: {
    suggested_path: { type: 'string', description: 'e.g. docs/feature-<name>.md' },
    markdown:       { type: 'string', description: 'the finished user feature doc' },
    changed_from_draft: { type: 'string', description: 'how the shipped behavior diverged from the design-time draft, if at all' },
  },
  required: ['markdown'],
}
const featureDoc = await agent(
  `Write the FINAL user feature doc for what this feature actually shipped — the operator's "what it does / how you use it" guide, NOT the maintainer's "why".
Request: ${request}
Design-time draft (intended behavior — reconcile against reality): ${design.feature_doc}
Fork answers: ${JSON.stringify(answers)}
Built: ${JSON.stringify(delegated.map((d) => ({ chunk: d.chunk, summary: d.impl && d.impl.summary, files: d.impl && d.impl.files })))}
Also shipped by the main loop (self/kit): ${JSON.stringify(mine.map((c) => c.chunk))}
Read an existing docs/feature-*.md and MATCH its voice and shape (tagline, what-it-is, setup/use, honest limitations). Read the changed files so the doc describes real behavior, not the draft's guesses. Second person, plain language, no decision-NNN/file references or internal jargon — apply the copywriting skill and run its linter to 0 hard:
  bash ~/.agents/skills/copywriting/scripts/check-copy.sh <the doc you drafted>
Return the finished markdown, a suggested docs/feature-<name>.md path, and how it diverged from the draft.`,
  { label: 'feature-doc', phase: 'Document', schema: DOC_SCHEMA },
)

return {
  design_of_record: design.design_of_record,
  feature_doc: featureDoc,  // finished user guide (markdown + suggested path) — write to docs/feature-<name>.md with the commit
  do_in_main_loop: mine,    // self + kit chunks: linchpins, migrations, kit manifests — write these yourself
  delegated,                // each: branch + build summary + adversarial verdict/findings
  needs_attention: suspect, // chunks a verifier could not clear — review before integrating
  polish,                   // the copy/quality edit list + linter result — APPLY before committing
  integration:
    'For each green, cleared branch: cherry-pick onto main and review the linchpins yourself. Apply the `polish.edits` (and copy-edit the self/kit chunks the same way). Run the COMBINED gate AND the copywriting linter to 0 hard. State-leak check, commit (conventional + trailers), push. Then write BOTH docs — the decision doc (from design_of_record) and the user feature doc (feature_doc.markdown → feature_doc.suggested_path) — plus memory. STOP — the live redeploy is the operator\'s.',
}
