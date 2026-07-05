package main

// Recording and replaying message-log cassettes (docs/13): a green run's
// log is captured once, committed, and replayed against every future build
// to prove old logs still fold — the graduation loop's first rung.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhamidi/k-si/cassette"
	"github.com/dhamidi/k-si/runtime"
)

const cassetteDir = "t/cassettes/logs"

func recordCassette(script string, log runtime.Log) error {
	if err := os.MkdirAll(cassetteDir, 0o755); err != nil {
		return err
	}

	slug := strings.TrimSuffix(strings.TrimPrefix(script, "t/"), ".test")
	slug = strings.ReplaceAll(slug, "/", "-")
	path := filepath.Join(cassetteDir, slug+".jsonl")

	prov := cassette.Provenance{
		Kind:       "message-log",
		RecordedAt: time.Now().UTC(),
		RecordedBy: "kasi test --record",
		Source:     script,
	}

	if err := cassette.Save(path, prov, log); err != nil {
		return err
	}

	fmt.Printf("rec  %s\n", path)
	return nil
}

// runCassettes replays every committed log cassette against the current
// full assembly. The promise being tested is the open set's (docs/01): an
// old log folds without error on a new build — unknown tags drop, nothing
// crashes — and folding it twice lands on the same model.
func runCassettes() int {
	paths, err := filepath.Glob(filepath.Join(cassetteDir, "*.jsonl"))
	if err != nil || len(paths) == 0 {
		fmt.Printf("kasi test --cassettes: no cassettes under %s\n", cassetteDir)
		return 0
	}

	failed := 0

	for _, path := range paths {
		if err := replayCassette(path); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "FAIL %s\n%s\n", path, indent(err.Error()))
		} else {
			fmt.Printf("ok   %s\n", path)
		}
	}

	fmt.Printf("%d cassettes, %d failed\n", len(paths), failed)
	if failed > 0 {
		return 1
	}
	return 0
}

func replayCassette(path string) error {
	c, err := cassette.Load(path)
	if err != nil {
		return err
	}

	first := runtime.New(assembly()...).UseLog(c)
	if err := first.Replay(); err != nil {
		return fmt.Errorf("replay failed: %w", err)
	}

	second := runtime.New(assembly()...).UseLog(c)
	if err := second.Replay(); err != nil {
		return fmt.Errorf("second replay failed: %w", err)
	}

	for _, name := range first.ModuleNames() {
		a, _ := first.ModelJSON(name)
		b, _ := second.ModelJSON(name)
		if string(a) != string(b) {
			return fmt.Errorf("module %s: two folds of the same cassette disagree\n first:  %s\n second: %s", name, a, b)
		}
	}

	return nil
}
