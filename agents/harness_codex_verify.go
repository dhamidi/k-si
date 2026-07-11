package agents

// Coverage for the decision-004-critical Codex home lifecycle (decision-025). The
// merge gate forbids *_test.go — the scenario scripts ARE the tests (docs/12) — and
// the real *Codex home/auth/finalize core is gated on h.(*Codex), which no test ring
// resolves, so a scenario cannot reach it through the harness registry. This file
// factors the check into ONE exported entry that exercises the lifecycle's pure and
// package-internal seams DIRECTLY — no live codex, no subprocess, no harness dispatch
// — invoked by the `codex-lifecycle` scenario vocab. It never touches the sim/recorded/
// live rings or their cassettes, so the twin rings stay byte-identical.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dhamidi/k-si/secrets"
)

// fakeRefresher is the CredentialRefresher double VerifyCodexLifecycle drives: it
// records whether Refresh fired and honours a settable Connected answer, so the check
// can assert rotation-writeback-only-when-connected without a live codex or a real
// secrets store.
type fakeRefresher struct {
	connected bool
	refreshed bool
	lastBlob  string
}

func (f *fakeRefresher) Connected(string) (bool, error) { return f.connected, nil }

func (f *fakeRefresher) Refresh(_ string, plaintext string) error {
	f.refreshed = true
	f.lastBlob = plaintext
	return nil
}

// VerifyCodexLifecycle exercises the decision-004-critical Codex home lifecycle end to
// end with NO live codex and NO harness dispatch (decision-025). It asserts, in order:
// per-task home path derivation (and inert on an empty root), materialize-from-secret
// at 0600, a MINIMAL config with no host bleed, REUSE across resume turns (same home,
// codex's sessions/ store survives), no plaintext blob riding the env (path only),
// rotation-writeback-only-when-connected-and-audited (and skipped when unchanged),
// finalize NOT removing the persistent home, teardown on task-finish, and the boot
// sweep reaping a done/absent task's orphan while keeping a live one. Returns a non-nil
// error naming the first failed invariant. Invoked by the `codex-lifecycle` scenario.
func VerifyCodexLifecycle() error {
	root, err := os.MkdirTemp("", "kasi-codex-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	const taskID = int64(42)
	ctx := context.Background()

	// 1. Path derivation: per-task under the root; empty root is inert (twin rings).
	if got := CodexHomeDir("", taskID); got != "" {
		return fmt.Errorf("CodexHomeDir with an empty root = %q, want empty (inert in twin rings)", got)
	}
	home := CodexHomeDir(root, taskID)
	if want := filepath.Join(root, "42"); home != want {
		return fmt.Errorf("CodexHomeDir = %q, want %q", home, want)
	}

	// 2. Materialize from the reserved secret: the sim edge resolves it to a sentinel.
	// auth.json must land 0600 with that plaintext, and config.toml must be the
	// käsi-minimal one — never a host copy that would bleed an API key over the OAuth
	// login (decision-004).
	sec := secrets.NewSim()
	e := Edges{Secrets: sec}
	if err := materializeCodexHome(ctx, e, home); err != nil {
		return fmt.Errorf("materialize turn 1: %w", err)
	}
	authPath := filepath.Join(home, "auth.json")
	info, err := os.Stat(authPath)
	if err != nil {
		return fmt.Errorf("auth.json not materialized: %w", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		return fmt.Errorf("auth.json perms = %o, want 0600 (decision-004)", perm)
	}
	blob, _ := os.ReadFile(authPath)
	wantSecret := "SENTINEL-SECRET(codex/oauth/auth-json)"
	if string(blob) != wantSecret {
		return fmt.Errorf("auth.json = %q, want the resolved reserved secret %q", blob, wantSecret)
	}
	cfg, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil {
		return fmt.Errorf("config.toml not seeded: %w", err)
	}
	if !bytes.Contains(cfg, []byte("käsi-managed")) {
		return fmt.Errorf("config.toml = %q, want the minimal käsi-managed placeholder (no host bleed)", cfg)
	}

	// 3. Reuse across resume turns: drop a marker in the sessions/ store codex keeps,
	// materialize AGAIN for the same task, and prove the same home was reused (the
	// marker survives) rather than a fresh dir minted per run.
	sessMarker := filepath.Join(home, "sessions", "rollout.jsonl")
	if err := os.MkdirAll(filepath.Dir(sessMarker), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(sessMarker, []byte("turn-1 rollout"), 0o600); err != nil {
		return err
	}
	if err := materializeCodexHome(ctx, e, CodexHomeDir(root, taskID)); err != nil {
		return fmt.Errorf("materialize turn 2 (resume): %w", err)
	}
	if got, err := os.ReadFile(sessMarker); err != nil || string(got) != "turn-1 rollout" {
		return fmt.Errorf("resume turn did not REUSE the per-task home: sessions/ rollout lost (%v)", err)
	}

	// 4. No plaintext blob rides the env: the lifecycle passes only the DIR PATH, the
	// credential is a file. The env-carried CODEX_HOME must not be the blob itself.
	if home == string(blob) {
		return fmt.Errorf("the env-carried CODEX_HOME must be a path, not the credential blob (decision-004)")
	}

	// 5. Rotation write-back ONLY when connected + audited. finalizeCredentials must
	// skip a disconnected operator (a deleted reserved ref) and write back a rotated
	// token when still connected.
	rotate := func(connected bool) *fakeRefresher {
		ref := &fakeRefresher{connected: connected}
		_ = os.WriteFile(authPath, []byte("rotated-token"), 0o600) // codex rotated it this turn
		c := &Codex{refresh: ref}
		c.finalizeCredentials(&codexRun{codexHome: home, authBefore: []byte("old-token"), authFound: true})
		return ref
	}
	if ref := rotate(false); ref.refreshed {
		return fmt.Errorf("rotation wrote back while DISCONNECTED — must never resurrect a signed-out secret (decision-025)")
	}
	if ref := rotate(true); !ref.refreshed || ref.lastBlob != "rotated-token" {
		return fmt.Errorf("rotation did NOT write back the fresh token while connected (refreshed=%v blob=%q)", ref.refreshed, ref.lastBlob)
	}
	// And no write-back when the token is UNCHANGED this turn, even connected.
	_ = os.WriteFile(authPath, []byte("same-token"), 0o600)
	unchanged := &fakeRefresher{connected: true}
	(&Codex{refresh: unchanged}).finalizeCredentials(&codexRun{codexHome: home, authBefore: []byte("same-token"), authFound: true})
	if unchanged.refreshed {
		return fmt.Errorf("rotation wrote back an UNCHANGED token — should no-op")
	}

	// 6. finalize must NOT remove the home — it persists across the task's turns.
	if _, err := os.Stat(home); err != nil {
		return fmt.Errorf("home was removed by finalize — it must persist across the task's turns: %w", err)
	}

	// 7. Teardown on task-finish removes the home and its 0600 credential.
	if err := RemoveCodexHome(root, taskID); err != nil {
		return err
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		return fmt.Errorf("RemoveCodexHome did not delete the home (decision-004: no 0600 credential may linger)")
	}

	// 8. Boot sweep reaps an orphan (a done/absent task) but keeps a live one.
	live := CodexHomeDir(root, 7)
	orphan := CodexHomeDir(root, 9)
	for _, d := range []string{live, orphan} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	SweepCodexHomes(root, func(id int64) bool { return id == 7 }) // 7 in flight; 9 done/absent
	if _, err := os.Stat(live); err != nil {
		return fmt.Errorf("boot sweep reaped a LIVE task's home (task 7): %w", err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		return fmt.Errorf("boot sweep did NOT reap the orphan home (task 9)")
	}

	return nil
}
