package main

// `kasi secret` — the CLI face of the secrets store (docs/06). Until the web
// settings UI exists (stage 3), this is how standing credentials are seeded:
//
//   fnox get FASTMAIL_API_KEY | kasi secret set secret://fastmail/api-token
//   kasi secret ls
//
// The value is read from STDIN, never an argv (which would land in shell
// history and the process table). Nothing here ever prints a value.

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhamidi/k-si/secrets"
)

func runSecret(args []string) int {
	flags := flag.NewFlagSet("kasi secret", flag.ExitOnError)
	state := flags.String("state", "data", "state directory holding the databases (docs/03)")
	flags.Parse(args)

	rest := flags.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "usage: kasi secret <set|ls> [args]")
		return 2
	}

	if err := os.MkdirAll(*state, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "kasi secret:", err)
		return 1
	}
	key, err := secrets.LoadKey(*state)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kasi secret:", err)
		return 1
	}
	store, err := secrets.OpenSQLite(filepath.Join(*state, "secrets.db"), key)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kasi secret:", err)
		return 1
	}
	defer store.Close()

	switch rest[0] {
	case "set":
		if len(rest) != 2 {
			fmt.Fprintln(os.Stderr, "usage: kasi secret set <secret://namespace/key>   (value on stdin)")
			return 2
		}
		value, err := readSecretValue(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "kasi secret:", err)
			return 1
		}
		if value == "" {
			fmt.Fprintln(os.Stderr, "kasi secret set: no value on stdin")
			return 1
		}
		if err := store.Set(rest[1], value); err != nil {
			fmt.Fprintln(os.Stderr, "kasi secret:", err)
			return 1
		}
		fmt.Printf("set %s\n", rest[1]) // the reference, never the value
		return 0

	case "ls":
		urls, err := store.List()
		if err != nil {
			fmt.Fprintln(os.Stderr, "kasi secret:", err)
			return 1
		}
		for _, url := range urls {
			fmt.Println(url)
		}
		return 0

	default:
		fmt.Fprintf(os.Stderr, "kasi secret: unknown command %q (set, ls)\n", rest[0])
		return 2
	}
}

// readSecretValue reads the whole of stdin and trims a single trailing newline,
// so a piped `fnox get …` (which appends one) round-trips exactly.
func readSecretValue(r io.Reader) (string, error) {
	b, err := io.ReadAll(bufio.NewReader(r))
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\n"), nil
}
