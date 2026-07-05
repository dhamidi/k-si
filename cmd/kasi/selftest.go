package main

// The test-language conformance corpus (docs/12, BUILDING stage 0): the
// runner cannot be tested by scripts it interprets, so these scripts carry
// their expected outcome. pass/ scripts must succeed; fail/ scripts must
// fail, optionally with `#! expect-fail <substring>` naming the failure.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runSelftest() int {
	failed := 0

	failed += selftestDir("testlang/corpus/pass", func(name, src string) error {
		return runScript(src, "0")
	})

	failed += selftestDir("testlang/corpus/fail", func(name, src string) error {
		want := expectFailDirective(src)
		err := runScript(src, "0")

		if err == nil {
			return fmt.Errorf("expected failure, but the script passed")
		}
		if want != "" && !strings.Contains(err.Error(), want) {
			return fmt.Errorf("failed with %q, want a failure containing %q", err.Error(), want)
		}
		return nil
	})

	if failed > 0 {
		return 1
	}

	fmt.Println("selftest: conformance corpus green")
	return 0
}

func selftestDir(dir string, check func(name, src string) error) int {
	scripts, err := filepath.Glob(filepath.Join(dir, "*.test"))
	if err != nil || len(scripts) == 0 {
		fmt.Fprintf(os.Stderr, "selftest: no corpus in %s\n", dir)
		return 1
	}

	failed := 0
	for _, script := range scripts {
		src, err := os.ReadFile(script)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", script, err)
			failed++
			continue
		}

		if err := check(script, string(src)); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s\n%s\n", script, indent(err.Error()))
			failed++
		} else {
			fmt.Printf("ok   %s\n", script)
		}
	}

	return failed
}

func expectFailDirective(src string) string {
	first, _, _ := strings.Cut(src, "\n")
	if directive, ok := strings.CutPrefix(first, "#! expect-fail"); ok {
		return strings.TrimSpace(directive)
	}
	return ""
}
