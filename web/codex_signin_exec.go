package web

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// ExecCodexSignIn is the real Codex sign-in launcher (decision-025): it shells
// `codex login --device-auth` against a DEDICATED, käsi-managed home, so the
// sign-in neither clobbers a host login nor leaves the credential where käsi
// cannot harvest it. device-auth needs no inbound callback — codex polls OpenAI
// out and the operator approves in their own browser (host-gated, decision-006) —
// so the reachability wall that blocks inbound webhooks does not apply here.
//
// The device-auth home MUST already exist and carry a config.toml or codex falls
// back to its defaults, so Start makes the directory and seeds a minimal config
// before spawning. The credential is a ~4KB auth.json the web edge harvests once
// and drops (decision-004); it never rides an OS env var — only the home path
// does — so it never lands in the child's /proc environ.
type ExecCodexSignIn struct {
	// Binary is the codex executable (default "codex"); overridable so a host with
	// a non-default install path can point at it.
	Binary string
}

// NewExecCodexSignIn builds the real launcher production wires through
// SetCodexSignIn. binary defaults to "codex" when empty.
func NewExecCodexSignIn(binary string) *ExecCodexSignIn {
	if binary == "" {
		binary = "codex"
	}
	return &ExecCodexSignIn{Binary: binary}
}

// signInParseTimeout bounds how long Start waits for codex to print the code and
// URL before giving up — the device-auth prints them almost immediately.
const signInParseTimeout = 30 * time.Second

var (
	codexURLRE  = regexp.MustCompile(`https?://\S+`)
	codexCodeRE = regexp.MustCompile(`[A-Z0-9]{4,}-[A-Z0-9]{4,}`)
)

func (e *ExecCodexSignIn) Start(ctx context.Context, harvest CodexHarvest) (CodexSignInSession, error) {
	home, err := os.MkdirTemp("", "kasi-codex-signin-")
	if err != nil {
		return nil, fmt.Errorf("codex sign-in: make home: %w", err)
	}
	// codex falls back to its defaults unless the home carries a config.toml, so
	// seed a minimal one (0600 — the home also lands the credential).
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("# käsi-managed Codex sign-in home\n"), 0o600); err != nil {
		os.RemoveAll(home)
		return nil, fmt.Errorf("codex sign-in: seed config: %w", err)
	}

	cmd := exec.Command(e.Binary, "login", "--device-auth")
	cmd.Env = append(os.Environ(), "CODEX_HOME="+home)
	// Merge stderr into stdout: codex may print the code/URL to either.
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		os.RemoveAll(home)
		return nil, fmt.Errorf("codex sign-in: stdout: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	sess := &execCodexSession{cmd: cmd, home: home, harvest: harvest, scanDone: make(chan struct{})}
	if err := cmd.Start(); err != nil {
		os.RemoveAll(home)
		return nil, fmt.Errorf("codex sign-in: start: %w", err)
	}

	// One scanner harvests the public code and URL and drains the pipe to EOF; one
	// waiter reaps the process. os/exec requires every read from StdoutPipe to reach
	// EOF BEFORE Wait() (Wait closes the pipe on exit), so reap blocks on the
	// scanner's completion before it calls Wait — never the two racing.
	ready := make(chan struct{})
	go sess.scan(pipe, ready)
	go sess.reap()

	select {
	case <-ready:
		return sess, nil
	case <-time.After(signInParseTimeout):
		sess.Close()
		return nil, fmt.Errorf("codex sign-in: timed out reading the sign-in code")
	case <-ctx.Done():
		sess.Close()
		return nil, ctx.Err()
	}
}

// execCodexSession tracks one running device-auth: the process, its dedicated
// home, the harvested public code/URL, and whether it has exited (and how).
type execCodexSession struct {
	cmd  *exec.Cmd
	home string

	// harvest stores the credential the moment the subprocess exits — never on a
	// poll GET (decision-004, decision-025). reap reads auth.json at the process
	// edge and hands it straight to this callback; the poll edge only reads the
	// resulting state.
	harvest CodexHarvest

	// scanDone closes when scan has drained the pipe to EOF, so reap can call Wait
	// only after all reads have completed (os/exec's ordering requirement).
	scanDone chan struct{}

	mu       sync.Mutex
	code     string
	url      string
	exited   bool
	exitedOK bool
	closed   bool
}

// scan reads the process output line by line, records the first URL and code it
// sees, and closes ready once both are known so Start can return the display
// values. It drains the rest so the pipe never blocks the child, and closes
// scanDone once it hits EOF so reap knows every read has completed before Wait.
func (s *execCodexSession) scan(r io.Reader, ready chan<- struct{}) {
	defer close(s.scanDone)
	signalled := false
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		s.mu.Lock()
		if s.url == "" {
			if m := codexURLRE.FindString(line); m != "" {
				s.url = m
			}
		}
		if s.code == "" {
			if m := codexCodeRE.FindString(line); m != "" {
				s.code = m
			}
		}
		haveBoth := s.url != "" && s.code != ""
		s.mu.Unlock()
		if haveBoth && !signalled {
			signalled = true
			close(ready)
		}
	}
}

// reap waits for the process and records its outcome, so Poll never blocks. It
// blocks on scanDone first: os/exec forbids Wait before every StdoutPipe read has
// reached EOF, and scan closes scanDone exactly then. On a clean exit it harvests
// the credential server-side — reads auth.json at the process edge and hands it
// straight to the store through the harvest callback (decision-004) — so the poll
// GET only ever READS the resulting state, never writing the store. A failed
// process, an unreadable auth.json, or a store error all record a failed sign-in
// the operator retries; the reference alone is logged, never the blob.
func (s *execCodexSession) reap() {
	<-s.scanDone
	err := s.cmd.Wait()
	ok := err == nil
	if ok && s.harvest != nil {
		blob, rerr := os.ReadFile(filepath.Join(s.home, "auth.json"))
		if rerr != nil {
			log.Printf("web: codex: read harvested credential: %v", rerr)
			ok = false
		} else if herr := s.harvest(blob); herr != nil {
			ok = false
		}
	}
	s.mu.Lock()
	s.exited = true
	s.exitedOK = ok
	s.mu.Unlock()
}

func (s *execCodexSession) Code() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.code
}

func (s *execCodexSession) VerificationURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.url
}

func (s *execCodexSession) Poll() CodexSignInState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.exited {
		return CodexSignInWaiting
	}
	if s.exitedOK {
		return CodexSignInDone
	}
	return CodexSignInFailed
}

// Close terminates the process if it is still running and removes the dedicated
// home, taking the harvested credential file with it.
func (s *execCodexSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	exited := s.exited
	home := s.home
	s.mu.Unlock()

	if !exited && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	return os.RemoveAll(home)
}
