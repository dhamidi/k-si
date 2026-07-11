package agents

// The harness registry (decision-024): käsi shells out to an official agent
// harness rather than implementing an agent loop, and an operator may choose
// WHICH one per task. Edges carries a map of harnesses keyed by name; each run is
// pinned to one name for its whole life, and every edge call dispatches through
// the map by that name so a restart resolves the SAME harness that launched.

// DefaultHarness is the built-in harness a run resolves to when its pin is empty —
// the unset sentinel every legacy run and default deployment carries. Keeping the
// default at "claude" is what makes the new field marshal-stable: an unpinned run
// omits Harness, so its log stays byte-identical to before the registry existed.
const DefaultHarness = "claude"

// HarnessNames lists the harnesses an operator may choose between, in the order
// the worker_harness setting offers them (decision-024). It is the canonical
// selectable set — the boot registry may register the same names over the twins or
// omit an uninstalled real adapter, but the choice a person makes is always from
// this list.
func HarnessNames() []string {
	return []string{"claude", "codex"}
}

// OverEveryName registers one harness under every selectable name — how the
// sim, recorded, and recording twins enter the registry (decision-024). They are
// harness-agnostic decorators over the single Harness interface, so a scenario
// pinning any name dispatches to the same twin and the conformance suite runs
// unchanged over "codex" as over "claude".
func OverEveryName(h Harness) map[string]Harness {
	m := make(map[string]Harness, len(HarnessNames()))
	for _, name := range HarnessNames() {
		m[name] = h
	}
	return m
}

// harnessName resolves a run's pin to a concrete harness name, defaulting the
// empty unset sentinel to the built-in harness so legacy runs and replays converge
// on Claude.
func harnessName(name string) string {
	if name == "" {
		return DefaultHarness
	}
	return name
}

// resolveHarness looks the run's pinned harness out of the registry, defaulting an
// empty pin to the built-in harness. A pinned-but-missing name (a harness removed
// from a deployment after a run was pinned to it) falls back to the default rather
// than nil so the edge call never panics — the run then errors at exec, which the
// finish path records, instead of crashing the process.
func (e Edges) resolveHarness(name string) Harness {
	if h, ok := e.Harnesses[harnessName(name)]; ok {
		return h
	}
	return e.Harnesses[DefaultHarness]
}
