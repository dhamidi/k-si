package main

// `kasi probe` is ring 3 (docs/13): it runs the live probes against the ACTUAL
// world — real Claude, real Fastmail — and refreshes the cassettes rings 1 and 2
// replay. It is never in the merge loop and never automated here; you trigger it
// deliberately, in an environment with credentials, when an edge changes or
// before a release. A green probe refreshes its cassette; a red one leaves the
// recording behind as a debugging artifact. The cassette diff it produces is the
// changelog of the outside world — review it with `git diff t/cassettes/`.

import (
	"flag"
	"fmt"
	"os"
)

func runProbe(args []string) int {
	flags := flag.NewFlagSet("kasi probe", flag.ExitOnError)
	dryRun := flags.Bool("dry-run", false, "list the probes and the cassettes they would refresh, without touching the world")
	flags.Parse(args)

	paths := flags.Args()
	if len(paths) == 0 {
		paths = []string{"t/recorded"}
	}

	scripts, err := collectScripts(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kasi probe:", err)
		return 1
	}
	if len(scripts) == 0 {
		fmt.Println("kasi probe: no probes found")
		return 0
	}

	if *dryRun {
		fmt.Printf("kasi probe: %d probe(s) would run live and refresh:\n", len(scripts))
		for _, s := range scripts {
			fmt.Printf("  %s\n", s)
			fmt.Printf("      harness: %s%s\n", harnessCassetteDir(s), existsNote(harnessCassetteDir(s)))
			fmt.Printf("      mail:    %s%s\n", mailCassetteDir(s), existsNote(mailCassetteDir(s)))
		}
		return 0
	}

	newLog, err := logFactory("memory")
	if err != nil {
		fmt.Fprintln(os.Stderr, "kasi probe:", err)
		return 1
	}

	// Ring 3 spends real money on real agents and sends real mail; say so loudly.
	fmt.Printf("kasi probe: running %d live probe(s) — real agents, real mail, real money.\n", len(scripts))

	failed := 0
	for _, script := range scripts {
		if err := runScriptFleet(script, 1, newLog, true, "live"); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "FAIL %s (recording left for debugging)\n%s\n", script, indent(err.Error()))
		} else {
			fmt.Printf("ok   %s (cassettes refreshed)\n", script)
		}
	}

	fmt.Printf("%d probe(s), %d failed\n", len(scripts), failed)
	if failed == 0 {
		fmt.Println("review the outside world's changes: git diff t/cassettes/")
	}
	if failed > 0 {
		return 1
	}
	return 0
}

// existsNote marks whether a cassette directory is already present, so a dry run
// shows what a probe would create versus refresh.
func existsNote(dir string) string {
	if _, err := os.Stat(dir); err == nil {
		return "  (refresh)"
	}
	return "  (new)"
}
