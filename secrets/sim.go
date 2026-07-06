package secrets

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// SimSecrets is the simulated credential edge (docs/13): it never holds real
// values. Each Resolve returns a unique SENTINEL string and remembers it, so the
// scenario runner can scan the log, the model, and the content tables for any
// sentinel — a hit means a secret leaked out of an effect into somewhere it must
// never be (docs/06 invariants). The runner keeps one across a simulated crash.
type SimSecrets struct {
	mu     sync.Mutex
	issued map[string]string // url -> sentinel
}

var _ Secrets = (*SimSecrets)(nil)

func NewSim() *SimSecrets {
	return &SimSecrets{issued: map[string]string{}}
}

// Set records that a secret exists at url and DISCARDS the plaintext — the sim
// edge never holds a real value (docs/06). It mirrors *SQLiteSecrets.Set so the
// web edge can call either; the plaintext argument is deliberately dropped, so
// nothing the log or a message can reach ever sees it. After Set, Resolve(url)
// returns the url's sentinel and Issued() lists it, exactly as if it had been
// resolved — enough for a scenario to assert a reference was stored.
func (s *SimSecrets) Set(url, plaintext string) error {
	ns, key, err := parseURL(url)
	if err != nil {
		return err
	}
	_ = plaintext // discarded: the sim edge never stores a real value

	s.mu.Lock()
	defer s.mu.Unlock()

	s.issued[url] = fmt.Sprintf("SENTINEL-SECRET(%s/%s)", ns, key)
	return nil
}

// Resolve returns a deterministic sentinel for a valid secret:// URL and records
// it as issued. Deterministic so replay converges; recorded so leaks are found.
func (s *SimSecrets) Resolve(ctx context.Context, url string) (string, error) {
	ns, key, err := parseURL(url)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sentinel := fmt.Sprintf("SENTINEL-SECRET(%s/%s)", ns, key)
	s.issued[url] = sentinel
	return sentinel, nil
}

// Issued returns every sentinel handed out so far, sorted — the set the runner
// scans durable state for (docs/13).
func (s *SimSecrets) Issued() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, 0, len(s.issued))
	for _, sentinel := range s.issued {
		out = append(out, sentinel)
	}
	sort.Strings(out)
	return out
}
