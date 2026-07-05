// kasi is the single binary (docs/09): `kasi serve` runs the system,
// `kasi test` runs the scenario suite, and the control subcommands arrive
// with the control interface (docs/11).
//
// main.go is THE assembly point (docs/01): the full module list, in the
// open. If a module isn't named here, it isn't in the program.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/dhamidi/k-si/counter"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/store"
)

// assembly is the one module list. serve wires real edges; the test runner
// assembles the same list with each module's simulated set (docs/12).
func assembly(sim bool) []*runtime.Module {
	if sim {
		return []*runtime.Module{
			counter.Module(counter.SimEdges()),
		}
	}

	return []*runtime.Module{
		counter.Module(counter.Edges{Clock: runtime.RealClock{}}),
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
		os.Exit(runServe())
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: kasi <command>

Commands:
  serve   run käsi (stage 1+: real edges land per BUILDING.md)
  test    run test scripts: kasi test [-n N] [--ring sim] [--selftest] [path ...]`)
}

func runServe() int {
	// TODO(stage 1): open the SQLite log and content stores (docs/03) and
	// wire real edges here. Until then serve runs the assembly on the
	// in-memory twin so the skeleton stays runnable.
	app := runtime.New(assembly(false)...).UseLog(store.NewMemoryLog())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := app.Start(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "kasi serve:", err)
		return 1
	}

	fmt.Println("kasi: running (in-memory log until stage 1 — see BUILDING.md); Ctrl-C to stop")
	<-ctx.Done()
	app.Stop()
	return 0
}
