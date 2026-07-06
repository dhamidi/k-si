// kasi is the single binary (docs/09): `kasi serve` runs the system,
// `kasi test` runs the scenario suite, and the control subcommands arrive
// with the control interface (docs/11).
//
// main.go is THE assembly point (docs/01): the full module list, in the
// open. If a module isn't named here, it isn't in the program.
package main

import (
	"fmt"
	"github.com/dhamidi/k-si/skills"
	"os"

	"github.com/dhamidi/k-si/agents"
	"github.com/dhamidi/k-si/counter"
	"github.com/dhamidi/k-si/email"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/tasks"
)

// assembly is the module list wired to each module's simulated edge set — used
// by cassette replay and the runner's replay-convergence refold, which only fold
// the log and never drive an effect. `kasi serve` builds the same modules with
// real edges (serve.go); `kasi test` scenarios build them against a shared sim
// world (simworld.go). main.go stays the one place modules are named (docs/01).
func assembly() []*runtime.Module {
	return []*runtime.Module{
		skills.Module(skills.SimEdges()),
		counter.Module(counter.SimEdges()),
		email.Module(email.SimEdges()),
		tasks.Module(tasks.SimEdges()),
		agents.Module(agents.SimEdges()),
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "test":
		os.Exit(runTest(os.Args[2:]))
	case "serve":
		os.Exit(runServe(os.Args[2:]))
	case "secret":
		os.Exit(runSecret(os.Args[2:]))
	case "probe":
		os.Exit(runProbe(os.Args[2:]))
	case "capture-inbox":
		os.Exit(runCaptureInbox(os.Args[2:]))
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: kasi <command>

Commands:
  serve   run käsi: kasi serve [-addr 127.0.0.1:8787] [-state ./data]
  test    run test scripts: kasi test [-n N] [--log memory|sqlite] [--record] [--cassettes] [--selftest] [path ...]
  secret  manage the secrets store: kasi secret <set secret://ns/key | ls> [-state ./data]  (set reads the value from stdin)
  probe   run live ring-3 probes and refresh their cassettes: kasi probe [--dry-run] [path ...]  (spends real money — real agents and mail)
  capture-inbox  capture REAL inbound mail into the parse corpus: kasi capture-inbox [-n 10] [-state ./data] [-dir t/fixtures/mime]  (reads the live inbox, read-only)`)
}
