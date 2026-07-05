// kasi is the single binary (docs/09): `kasi serve` runs the system,
// `kasi test` runs the scenario suite, and the control subcommands arrive
// with the control interface (docs/11).
//
// main.go is THE assembly point (docs/01): the full module list, in the
// open. If a module isn't named here, it isn't in the program.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/dhamidi/k-si/agents"
	"github.com/dhamidi/k-si/counter"
	"github.com/dhamidi/k-si/email"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/store"
	"github.com/dhamidi/k-si/tasks"
	"github.com/dhamidi/k-si/web"
)

// assembly is the one module list. serve wires real edges; the test runner
// assembles the same list with each module's simulated set (docs/12).
func assembly(sim bool) []*runtime.Module {
	if sim {
		return []*runtime.Module{
			counter.Module(counter.SimEdges()),
			email.Module(email.SimEdges()),
			tasks.Module(tasks.SimEdges()),
			agents.Module(agents.SimEdges()),
		}
	}

	clock := runtime.RealClock{}
	return []*runtime.Module{
		counter.Module(counter.Edges{Clock: clock}),
		email.Module(email.Edges{Clock: clock}),
		tasks.Module(tasks.Edges{Clock: clock}),
		agents.Module(agents.Edges{Clock: clock}),
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
  secret  manage the secrets store: kasi secret <set secret://ns/key | ls> [-state ./data]  (set reads the value from stdin)`)
}

func runServe(args []string) int {
	flags := flag.NewFlagSet("kasi serve", flag.ExitOnError)
	addr := flags.String("addr", "127.0.0.1:8787", "listen address (host-gated deployment, docs/08)")
	state := flags.String("state", "data", "state directory holding the SQLite databases (docs/03)")
	flags.Parse(args)

	if err := os.MkdirAll(*state, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "kasi serve:", err)
		return 1
	}

	log, err := store.OpenSQLiteLog(filepath.Join(*state, "kasi.db"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "kasi serve:", err)
		return 1
	}
	defer log.Close()

	app := runtime.New(assembly(false)...).UseLog(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := app.Start(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "kasi serve:", err)
		return 1
	}
	defer app.Stop()

	server, err := web.NewServer(app)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kasi serve:", err)
		return 1
	}

	httpServer := &http.Server{Addr: *addr, Handler: server}
	go func() {
		<-ctx.Done()
		httpServer.Close()
	}()

	fmt.Printf("kasi: http://%s (state in %s); Ctrl-C to stop\n", *addr, *state)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "kasi serve:", err)
		return 1
	}
	return 0
}
