package main

// The `kasi test` runner: assembles instances from the one module list with
// simulated edges, registers the test vocabulary, runs scripts, and enforces
// the standing checks (docs/13, docs/14).

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/store"
	"github.com/dhamidi/k-si/testlang"
)

func runTest(args []string) int {
	flags := flag.NewFlagSet("kasi test", flag.ExitOnError)
	fleet := flags.Int("n", 1, "run N instances of each script concurrently")
	ring := flags.String("ring", "sim", "edge set: sim (recorded and live land per BUILDING.md)")
	logKind := flags.String("log", "memory", "log edge: memory (the twin) or sqlite (a real file per script)")
	record := flags.Bool("record", false, "on success, save each script's log as a cassette under t/cassettes/logs/")
	cassettes := flags.Bool("cassettes", false, "replay every committed log cassette against the current build")
	selftest := flags.Bool("selftest", false, "run the test-language conformance corpus")
	flags.Parse(args)

	if *ring != "sim" {
		fmt.Fprintf(os.Stderr, "kasi test: ring %q is not built yet — see BUILDING.md stage 2\n", *ring)
		return 1
	}

	if *selftest {
		return runSelftest()
	}

	if *cassettes {
		return runCassettes()
	}

	newLog, err := logFactory(*logKind)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kasi test:", err)
		return 1
	}

	paths := flags.Args()
	if len(paths) == 0 {
		paths = []string{"t"}
	}

	scripts, err := collectScripts(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kasi test:", err)
		return 1
	}

	if len(scripts) == 0 {
		fmt.Println("kasi test: no scripts found")
		return 0
	}

	failed := 0
	start := time.Now()

	for _, script := range scripts {
		if err := runScriptFleet(script, *fleet, newLog, *record); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "FAIL %s\n%s\n", script, indent(err.Error()))
		} else {
			fmt.Printf("ok   %s\n", script)
		}
	}

	fmt.Printf("%d scripts, %d failed, %s\n", len(scripts), failed, time.Since(start).Round(time.Millisecond))
	if failed > 0 {
		return 1
	}
	return 0
}

func collectScripts(paths []string) ([]string, error) {
	var scripts []string

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}

		if !info.IsDir() {
			scripts = append(scripts, p)
			continue
		}

		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".test") {
				scripts = append(scripts, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Strings(scripts)
	return scripts, nil
}

func runScriptFleet(path string, n int, newLog logMaker, record bool) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if n <= 1 {
		inst, err := runScript(string(src), "0", newLog)
		if err != nil {
			return err
		}
		if record {
			return recordCassette(path, inst.log)
		}
		return nil
	}

	errs := make([]error, n)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = runScript(string(src), strconv.Itoa(i), newLog)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("instance %d of %d: %w", i, n, err)
		}
	}
	return nil
}

// logMaker builds one script's log edge: the in-memory twin, or a real
// SQLite file so the same scenarios also prove the real store (docs/13).
type logMaker func() (runtime.Log, func(), error)

func logFactory(kind string) (logMaker, error) {
	switch kind {
	case "memory":
		return func() (runtime.Log, func(), error) {
			return store.NewMemoryLog(), func() {}, nil
		}, nil
	case "sqlite":
		return func() (runtime.Log, func(), error) {
			dir, err := os.MkdirTemp("", "kasi-test-*")
			if err != nil {
				return nil, nil, err
			}
			log, err := store.OpenSQLiteLog(filepath.Join(dir, "kasi.db"))
			if err != nil {
				os.RemoveAll(dir)
				return nil, nil, err
			}
			return log, func() { log.Close(); os.RemoveAll(dir) }, nil
		}, nil
	default:
		return nil, fmt.Errorf("unknown log kind %q (memory, sqlite)", kind)
	}
}

// instance is one script's world: a log that survives crashes, an assembly
// subset, and the current App.
type instance struct {
	log     runtime.Log
	cleanup func()
	newLog  logMaker
	clock   *runtime.SimulatedClock
	app     *runtime.App
	only    []string // nil = the full assembly
}

func (i *instance) full() bool { return i.only == nil }

func (i *instance) boot() error {
	mods := assembly(true)

	if i.only != nil {
		var subset []*runtime.Module
		for _, m := range mods {
			for _, name := range i.only {
				if m.Name() == name {
					subset = append(subset, m)
				}
			}
		}
		if len(subset) != len(i.only) {
			return fmt.Errorf("use: unknown module in %v (assembly has %v)", i.only, moduleNames(mods))
		}
		mods = subset
	}

	i.app = runtime.New(mods...).UseLog(i.log).UseClock(i.clock)
	return i.app.Start(context.Background())
}

func moduleNames(mods []*runtime.Module) []string {
	var names []string
	for _, m := range mods {
		names = append(names, m.Name())
	}
	return names
}

func runScript(src, index string, newLog logMaker) (*instance, error) {
	cmds, err := testlang.Parse(src)
	if err != nil {
		return nil, err
	}

	log, cleanup, err := newLog()
	if err != nil {
		return nil, err
	}

	inst := &instance{log: log, cleanup: cleanup, newLog: newLog, clock: runtime.SimClock()}
	if err := inst.boot(); err != nil {
		cleanup()
		return nil, err
	}
	defer func() {
		if inst.app != nil {
			inst.app.Stop()
		}
		inst.cleanup()
	}()

	in := testlang.New()
	in.Vars["I"] = index
	registerVocabulary(in, inst)

	if err := in.Run(cmds); err != nil {
		return inst, fmt.Errorf("%w\n%s", err, diagnostics(inst))
	}

	return inst, standingChecks(inst)
}

// standingChecks run after every script, so they can never be forgotten
// (docs/13): no dead sends in a full assembly, and replay convergence —
// folding the log from zero must rebuild exactly the live model.
func standingChecks(inst *instance) error {
	inst.app.Settle()

	if inst.full() {
		if dropped := inst.app.Dropped(); len(dropped) > 0 {
			return fmt.Errorf("dead sends: a full assembly dropped %v — a complete build has no business aiming messages at nothing (docs/13)\n%s",
				dropped, diagnostics(inst))
		}
	}

	mods := assembly(true)
	if inst.only != nil {
		var subset []*runtime.Module
		for _, m := range mods {
			for _, name := range inst.only {
				if m.Name() == name {
					subset = append(subset, m)
				}
			}
		}
		mods = subset
	}

	refold := runtime.New(mods...).UseLog(inst.log)
	if err := refold.Replay(); err != nil {
		return fmt.Errorf("replay convergence: refold failed: %w", err)
	}

	for _, name := range inst.app.ModuleNames() {
		live, _ := inst.app.ModelJSON(name)
		folded, _ := refold.ModelJSON(name)
		if !bytes.Equal(live, folded) {
			return fmt.Errorf("replay convergence: module %s diverged\n live:   %s\n refold: %s\n%s",
				name, live, folded, diagnostics(inst))
		}
	}

	return nil
}

func diagnostics(inst *instance) string {
	var b strings.Builder

	b.WriteString("message log (tail):\n")
	for _, line := range logTail(inst.log, 12) {
		b.WriteString("  " + line + "\n")
	}

	if dropped := inst.app.Dropped(); len(dropped) > 0 {
		b.WriteString("dropped: " + strings.Join(dropped, ", ") + "\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// --- Vocabulary ---------------------------------------------------------------

func registerVocabulary(in *testlang.Interp, inst *instance) {
	v := in.Vocabulary

	v["use"] = func(in *testlang.Interp, args []string) (string, error) {
		if len(args) == 0 {
			return "", fmt.Errorf("use needs module names or *")
		}
		inst.app.Stop()
		inst.cleanup()
		log, cleanup, err := inst.newLog() // a new world, not a crash
		if err != nil {
			return "", err
		}
		inst.log, inst.cleanup = log, cleanup
		if len(args) == 1 && args[0] == "*" {
			inst.only = nil
		} else {
			inst.only = args
		}
		return "", inst.boot()
	}

	v["send"] = func(in *testlang.Interp, args []string) (string, error) {
		if len(args) < 1 || len(args) > 2 {
			return "", fmt.Errorf("send needs a tag and an optional payload block")
		}

		tag := args[0]
		payload := json.RawMessage(nil)

		if len(args) == 2 {
			raw, err := payloadFromBlock(in, args[1])
			if err != nil {
				return "", err
			}
			payload = raw
		}

		if err := inst.app.StrictDecode(tag, payload); err != nil {
			return "", err
		}

		inst.app.Send(runtime.Msg{Tag: tag, Payload: payload})
		inst.app.Settle()
		return "", nil
	}

	v["advance"] = func(in *testlang.Interp, args []string) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("advance needs a duration, e.g. 5m")
		}
		d, err := time.ParseDuration(args[0])
		if err != nil {
			return "", err
		}
		inst.clock.Advance(d)
		inst.app.Settle()
		return "", nil
	}

	v["crash"] = func(in *testlang.Interp, args []string) (string, error) {
		// Drop the model and goroutines; keep only what production would
		// keep — the log (docs/01).
		inst.app.Stop()
		return "", nil
	}

	v["restart"] = func(in *testlang.Interp, args []string) (string, error) {
		if err := inst.boot(); err != nil {
			return "", err
		}
		inst.app.Settle()
		return "", nil
	}

	v["model"] = func(in *testlang.Interp, args []string) (string, error) {
		if len(args) == 0 {
			return "", fmt.Errorf("model needs a module name")
		}

		read, verb := splitVerb(args)
		raw, err := inst.app.ModelJSON(read[0])
		if err != nil {
			return "", err
		}

		value, err := walkJSON(raw, read[1:])
		if err != nil {
			return "", fmt.Errorf("model %s: %w", strings.Join(read, " "), err)
		}

		return finishRead(fmt.Sprintf("model %s", strings.Join(read, " ")), value, verb)
	}

	v["commands"] = func(in *testlang.Interp, args []string) (string, error) {
		value := strings.Join(inst.app.DrainTrace(), " ")
		return finishRead("commands", value, args)
	}

	v["dropped"] = func(in *testlang.Interp, args []string) (string, error) {
		var tags []string
		for _, d := range inst.app.Dropped() {
			tags = append(tags, strings.SplitN(d, " ", 2)[0])
		}
		return finishRead("dropped", strings.Join(tags, " "), args)
	}
}

// splitVerb separates a read's path from its trailing assertion verb.
func splitVerb(args []string) ([]string, []string) {
	for i, a := range args {
		if a == "is" || a == "are" || a == "matches" {
			return args[:i], args[i:]
		}
	}
	return args, nil
}

// finishRead makes every read double as an assertion (docs/14): bare, it
// returns its value for [ ] substitution; with a verb, it asserts.
func finishRead(what, value string, verb []string) (string, error) {
	if len(verb) == 0 {
		return value, nil
	}

	want := strings.Join(verb[1:], " ")

	switch verb[0] {
	case "is", "are":
		if value != want {
			return "", fmt.Errorf("%s is %q, want %q", what, value, want)
		}
	case "matches":
		ok, err := path.Match(want, value)
		if err != nil {
			return "", fmt.Errorf("bad pattern %q: %w", want, err)
		}
		if !ok {
			return "", fmt.Errorf("%s is %q, does not match %q", what, value, want)
		}
	default:
		return "", fmt.Errorf("unknown verb %q (is, are, matches)", verb[0])
	}

	return value, nil
}

// payloadFromBlock builds a message payload from a block of `field value`
// lines (docs/14). A braced value is a list; numbers and booleans are typed;
// substitution works inside the block.
func payloadFromBlock(in *testlang.Interp, block string) (json.RawMessage, error) {
	cmds, err := testlang.Parse(block)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{}

	for _, cmd := range cmds {
		if len(cmd.Words) != 2 {
			return nil, fmt.Errorf("line %d: a payload line is `field value` — quote strings with spaces, brace lists", cmd.Line)
		}

		key, err := in.EvalWord(cmd.Words[0])
		if err != nil {
			return nil, err
		}

		raw, err := in.EvalWord(cmd.Words[1])
		if err != nil {
			return nil, err
		}

		if cmd.Words[1].Braced {
			var list []any
			for _, item := range strings.Fields(raw) {
				list = append(list, scalar(item))
			}
			if list == nil {
				list = []any{}
			}
			payload[key] = list
		} else {
			payload[key] = scalar(raw)
		}
	}

	return json.Marshal(payload)
}

var numberPattern = regexp.MustCompile(`^-?[0-9]+$`)

func scalar(raw string) any {
	switch {
	case numberPattern.MatchString(raw):
		n, _ := strconv.ParseInt(raw, 10, 64)
		return n
	case raw == "true":
		return true
	case raw == "false":
		return false
	default:
		return raw
	}
}

// walkJSON descends a decoded slice by field names and renders the value:
// scalars plainly, lists space-joined, objects as compact JSON.
func walkJSON(raw []byte, path []string) (string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}

	for _, key := range path {
		obj, ok := value.(map[string]any)
		if !ok {
			return "", fmt.Errorf("%q is not an object", key)
		}
		value, ok = obj[key]
		if !ok {
			return "", fmt.Errorf("no field %q (have: %s)", key, strings.Join(keysOf(obj), ", "))
		}
	}

	return render(value), nil
}

func keysOf(obj map[string]any) []string {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func render(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'g', -1, 64)
	case []any:
		var parts []string
		for _, item := range v {
			parts = append(parts, render(item))
		}
		return strings.Join(parts, " ")
	default:
		raw, _ := json.Marshal(v)
		return string(raw)
	}
}

// logTail renders the last n log entries for failure output.
func logTail(log runtime.Log, n int) []string {
	var lines []string

	log.Replay(func(msg runtime.Msg, meta runtime.Meta) error {
		payload := string(msg.Payload)
		if payload == "" || payload == "null" {
			payload = "{}"
		}
		lines = append(lines, fmt.Sprintf("%4d %s %s", meta.Offset, msg.Tag, payload))
		return nil
	})

	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n")
}
