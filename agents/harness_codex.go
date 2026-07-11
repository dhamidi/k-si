package agents

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/dhamidi/k-si/secrets"
)

// codexAuthRef is the reserved reference the operator's Codex credential lives
// under (decision-025) — the same reference the /codex sign-in stores it at. The
// per-run CODEX_HOME materializes auth.json FROM this reference and, when codex
// rotates the token mid-turn, writes the fresh one back TO it, all at the edge so
// the plaintext never touches the model or the log (decision-004).
var codexAuthRef = secrets.URL("codex/oauth", "auth-json")

// CredentialRefresher persists a rotated Codex credential back to its reserved
// secret reference (decision-025). It is the secrets edge narrowed to a single
// Set, so the harness can write back a token codex rotated mid-run WITHOUT the
// plaintext ever entering the model or the log (decision-004). It is written
// directly, never via a message, so no per-run log entry is emitted and the replay
// and cassette rings stay byte-identical. nil where there is nothing real to
// refresh (the sim/recorded/live-capture rings resolve a twin, not the real Codex).
type CredentialRefresher interface {
	Set(ref, plaintext string) error
}

// codexPreamble leads the prompt on a Codex RESUME turn — the Codex equivalent of
// resumePreamble (decision-019). A resumed Codex session still holds the prior
// turn's transcript, which may end in "task complete", so without this the agent
// tends to re-affirm it is done and write no new reply.txt; the harvest then
// re-sends the prior reply. This makes the new turn explicit. It is prepended to
// codexPrompt so the standing file-contract still travels with every turn.
const codexPreamble = `A NEW message has arrived in this ongoing task. This is a FRESH turn in a
continuing conversation — NOT a review of finished work. Whatever you did on an
earlier turn is already done and its reply was already sent; do not repeat it, and
do not decide the task is complete just because earlier work is finished. Read
./in/body.txt for what the user is asking NOW and do that work. ./out/ has been
emptied for this turn, so you MUST write a fresh ./out/reply.txt (or ./out/
request.json) for anything to be sent back — if you write nothing, the user hears
nothing. Then stop.

--- standing instructions (unchanged every turn) ---

`

// codexPrompt is käsi's standing instruction to a Codex worker. It mirrors
// workerPrompt's file contract (./in/ inputs → ./out/ reply and attachments,
// requests, notify, store, memory, apps) but avoids Claude-specific phrasing so it
// reads correctly to a Codex agent (the shared prompt leaks .claude/skills, which a
// Codex run does not use — decision-024, skills-parity fork). The out/ reply
// contract and in/MEMORY.md memory contract are identical across harnesses.
const codexPrompt = `You are käsi's worker agent, running headless in a task workspace.

Your inputs are in ./in/ — read ./in/body.txt (the message to act on; it opens
with the Subject, From, and Date, then a blank line and the message body) and any
other files there (attachments). Do the work the message asks for.

Write what should be sent back to the sender into ./out/:
  - ./out/reply.txt  — the reply body, plain text, always.
  - any other files you place in ./out/ become reply attachments.

If you need more from the user, you have two channels — pick by what you need:

  1. A plain question → write it into ./out/reply.txt and stop. They answer by
     email and you are resumed with their reply in ./in/.

  2. Structured fields, a file upload, or a SECRET (a password, API key, or
     one-time login) the user must NOT paste into an email → raise a web form.
     Write ./out/request.json: a JSON array of fields, each
       { "name": "...", "label": "...", "type": "...", "required": true|false }
     where type is one of text | longtext | choice | file | secret
     (a "choice" field also takes "options": ["...", ...]). Optionally also write
     ./out/reply.txt to explain the ask — käsi emails it with a secure link to the
     form. Then stop. When you are resumed:
       - text / longtext / choice answers and any uploaded files are in ./in/
         (the text answers as ./in/answers.json);
       - each requested SECRET is in your ENVIRONMENT, under a variable named
         exactly like the field. Secrets never appear in ./in/, a file, or the
         message history — read them only from the environment.

Use the form (not reply.txt) whenever a secret is involved: it is the only way to
collect one without it landing in an email.

To send the user a one-way message mid-task — something they need NOW that can't
wait for the reply and needs nothing back (e.g. a two-factor code with a countdown)
— run` + " `kasi notify \"your message\"`" + `. It emails them immediately and
returns at once, so you keep working in the same turn. Use a REPLY or a web-form
REQUEST when you need something back from the user; use notify when you don't.

./store/ is your private, persistent datastore — a real directory that PERSISTS
across tasks, unlike the rest of this workspace, which is wiped the moment a task
finishes. Keep SQLite databases, caches, and scratch files there, and read from it
before re-fetching anything you cached before. It is shared across all your tasks
but never shown to the user: it is your working state, not the user's memory.

When the right answer to a request is a small web app — a dashboard, a form, a
viewer — build it under ./store/<name>/ as a program that reads $PORT from its
environment and serves on it, then register it by running: kasi app add <name>.
käsi assigns a port, keeps it running, and lists it on its web UI at /apps; the
URL is returned so you can confirm it is live. Describe the app's HTTP operations
in a ./store/<name>/app.json so a later run can call them; kasi app rm <name>
stops and unregisters it.

Every app käsi is running is listed for you in ./in/apps.json — each app's local
URL and the operations it exposes — so while you work a task you can call an app
on localhost (e.g. curl its URL) instead of redoing work it already owns. An empty
object means nothing is registered yet.

./out/memory/ is the user-visible memory — the durable facts about the user and
their world (a preference, an account detail, a decision) that käsi shows and
curates on its web UI. This is DISTINCT from ./store/: store is your private
scratch; memory is knowledge the user sees. When the user tells you to remember
something, or you learn a fact worth keeping, write ./out/memory/<name>.md — the
file name is the fact's identity, so writing the same name again updates it. The
body is raw markdown, one fact per file; lead it with a short YAML "description:"
between --- fences and that line becomes the fact's summary in the index. Every
fact käsi knows is provisioned into each run as ./in/memory/<name>.md files plus a
./in/MEMORY.md index: read them for what it already knows. To FORGET a fact, delete
its ./in/memory/<name>.md. Names are flat slugs (no nested paths).

Never wait for input — always stop.`

// Codex is the OpenAI Codex harness adapter (decision-024): it shells out to the
// `codex` CLI, running one worker turn per task in the task's workspace, as a
// selectable alternative to the Claude harness. It is a second real twin of
// SimHarness — the same interface over a different subprocess — so nothing outside
// this file knows which harness is in use.
//
// Unlike Claude, Codex MINTS its own session id server-side and announces it on the
// first stdout line as {"type":"thread.started","thread_id":"..."} — käsi never
// supplies one. So Start HARVESTS that id off the stream and returns it on the
// Handle (recorded via record-session, decision-024), and Resume continues by the
// harvested id, falling back to `resume --last` when none was recorded yet. The CLI
// must be signed in with the operator's ChatGPT subscription in the process
// environment — käsi resolves no token into the model or the log (decision-004); the
// subscription auth rides the CLI's own logged-in credential, exactly as NewClaude
// assumes `claude` is authenticated.
type Codex struct {
	bin     string
	workdir string
	// refresh persists a token codex rotated mid-turn back to codexAuthRef at the
	// edge (decision-025). nil in the sim/recorded/live-capture rings — there is no
	// real credential to refresh — so finalizeCredentials only cleans up the home.
	refresh CredentialRefresher

	mu   sync.Mutex
	cond *sync.Cond
	runs map[int64]*codexRun // keyed by taskID
}

type codexRun struct {
	cmd   *exec.Cmd
	runID int64
	// session is the harvested thread id. It is seeded (empty for Start, the passed
	// id for Resume) and then OVERWRITTEN by the harvest goroutine the moment a
	// thread.started line is teed, so a resume that re-announces a (same or forked)
	// id updates it and one that announces none keeps the seed. sessMu guards it
	// because the goroutine writes while spawn reads.
	sessMu        sync.Mutex
	session       string
	transcriptRel string
	outDir        string
	done          chan error

	// codexHome is the transient per-run CODEX_HOME the effect materialized outside
	// the workspace (decision-025), or "" for every twin ring (no home was set). The
	// harness removes it after Wait; authBefore/authFound snapshot its auth.json at
	// spawn so a token codex rotates during the turn is detected and written back.
	codexHome  string
	authBefore []byte
	authFound  bool
}

var _ Harness = (*Codex)(nil)

// NewCodex builds the Codex harness over a workspace root ($WORKDIR). The `codex`
// CLI must be signed in with the operator's ChatGPT subscription in the process's
// environment (decision-004: the subscription credential is the CLI's own, never a
// token in the model or the log).
// refresh persists a token codex rotates mid-turn back to the reserved reference
// (decision-025); pass nil where there is no real credential store to write to (the
// sim and live-capture rings). In production it is the secrets edge, so the
// write-back happens at the edge, never through the model.
func NewCodex(workdir string, refresh CredentialRefresher) *Codex {
	c := &Codex{bin: "codex", workdir: workdir, refresh: refresh, runs: map[int64]*codexRun{}}
	c.cond = sync.NewCond(&c.mu)
	return c
}

// Start opens a NEW Codex session for a task's first turn (decision-024). Codex
// mints its own session id server-side, so we pass NO session flag and HARVEST the
// thread_id off the first stdout line, returning it on the Handle for record-session
// to persist. If the process dies before it announces one (the mint→record crash
// window), Start returns an empty session and record-session does not fire; the next
// Resume falls back to `resume --last`.
func (c *Codex) Start(ctx context.Context, taskID, runID int64, env map[string]string) (Handle, error) {
	return c.spawn(taskID, runID, "", false, false, env)
}

// Resume continues an existing Codex session for a later turn. session is the
// harvested id recorded on the run. When it is empty or the deterministic
// sessionFor placeholder — the mint→record crash window, where no real Codex id was
// ever recorded — no id can name a Codex thread, so Resume falls back to
// `codex exec resume --last`, which continues the newest session in the task's own
// workspace (cwd-scoped to task-<id> via cmd.Dir). Either way the resumed
// thread_id is harvested off the stream and record-session persists it for the next
// turn.
func (c *Codex) Resume(ctx context.Context, taskID, runID int64, session string, env map[string]string) (Handle, error) {
	last := session == "" || session == sessionFor(taskID)
	if last {
		session = "" // seed empty: a --last resume names no id up front, it harvests one
	}
	return c.spawn(taskID, runID, session, true, last, env)
}

func (c *Codex) spawn(taskID, runID int64, seed string, resume, last bool, env map[string]string) (Handle, error) {
	dir := filepath.Join(c.workdir, fmt.Sprintf("task-%d", taskID))
	if err := os.MkdirAll(filepath.Join(dir, "out"), 0o755); err != nil {
		return Handle{}, err
	}
	transcriptRel := fmt.Sprintf("transcript-%d.jsonl", runID)

	// Snapshot the per-run CODEX_HOME credential BEFORE the process can rotate it
	// (decision-025). The effect wrote auth.json into this private home and passed
	// only the DIR PATH through env; capturing the blob now lets finalizeCredentials
	// detect a mid-turn rotation and write it back. Empty home in every twin ring —
	// materialization is skipped unless the resolved harness is the real Codex — so
	// finalize is a pure no-op there and the cassette/replay rings are untouched.
	codexHome := env["CODEX_HOME"]
	var authBefore []byte
	var authFound bool
	if codexHome != "" {
		if blob, err := os.ReadFile(filepath.Join(codexHome, "auth.json")); err == nil {
			authBefore = blob
			authFound = true
		}
	}

	cmd, pipe, transcript, err := c.startProcess(dir, transcriptRel, seed, resume, last, env)
	if err != nil {
		return Handle{}, err
	}

	run := &codexRun{
		cmd:           cmd,
		runID:         runID,
		session:       seed, // seeded now; a harvested thread.started overwrites it
		transcriptRel: transcriptRel,
		outDir:        filepath.Join(dir, "out"),
		done:          make(chan error, 1),
		codexHome:     codexHome,
		authBefore:    authBefore,
		authFound:     authFound,
	}

	// sessionReady carries the first-known session up to spawn: the harvested
	// thread_id if Codex announces one, otherwise the seed once the stream ends.
	// Size 1 + sync.Once so exactly one value is delivered and the goroutine never
	// blocks on it.
	sessionReady := make(chan string, 1)
	var readyOnce sync.Once
	signalReady := func(id string) { readyOnce.Do(func() { sessionReady <- id }) }

	go func() {
		// Tee every stdout line VERBATIM to the transcript — including its trailing
		// newline — and parse each for thread.started. ReadBytes preserves exact
		// bytes and, unlike bufio.Scanner, has no 64KB line cap that would truncate a
		// large command_execution event, so the recorded cassette replays
		// byte-identically (decision-024, replay convergence).
		reader := bufio.NewReader(pipe)
		for {
			line, readErr := reader.ReadBytes('\n')
			if len(line) > 0 {
				_, _ = transcript.Write(line)
				if id, ok := threadStarted(line); ok {
					run.sessMu.Lock()
					run.session = id // OVERWRITE the seed with the announced id
					run.sessMu.Unlock()
					signalReady(id)
				}
			}
			if readErr != nil {
				break // EOF or a read error: the stream is over
			}
		}
		// The stream ended. If no thread.started was ever seen, release spawn with the
		// seed — empty for Start (the mint→record crash window) or the passed id for a
		// by-id Resume. A --last resume that announced nothing keeps its empty seed, so
		// record-session does not fire and the next turn resumes --last again.
		run.sessMu.Lock()
		sess := run.session
		run.sessMu.Unlock()
		signalReady(sess)

		waitErr := cmd.Wait()
		transcript.Close()
		run.done <- waitErr
	}()

	// Block until the session is known (thread.started teed, or the stream ended),
	// THEN register the run and return the harvested Handle so record-session gets
	// the real id (decision-024).
	harvested := <-sessionReady

	c.mu.Lock()
	c.runs[taskID] = run
	c.cond.Broadcast()
	c.mu.Unlock()
	return Handle{TaskID: taskID, RunID: runID, Session: harvested}, nil
}

// threadStarted reports the thread_id when line is Codex's session announcement
// {"type":"thread.started","thread_id":"..."} — the id käsi harvests instead of
// minting. Any non-JSON or other event line is skipped (returns false), keeping the
// harvest tolerant of the rest of the stream.
func threadStarted(line []byte) (string, bool) {
	var ev struct {
		Type     string `json:"type"`
		ThreadID string `json:"thread_id"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(line), &ev); err != nil {
		return "", false
	}
	if ev.Type == "thread.started" && ev.ThreadID != "" {
		return ev.ThreadID, true
	}
	return "", false
}

// startProcess builds and starts one codex worker process in dir. Codex mints and
// announces its own session, so no session flag is passed; käsi captures the raw
// event stream to the transcript for the harness-dispatched reader (decision-024,
// transcript fork) — verbatim as-received, like Claude's stream-json. A fresh turn
// runs `codex exec ... -C <dir> <prompt>`; a later turn runs the resume subcommand,
// `codex exec resume ... <session> <prompt>`, or `... --last <prompt>` when no id
// was recorded. resume takes NO -C, so its cwd is scoped to the task only via
// cmd.Dir. --skip-git-repo-check is required because task workspaces are not git
// repos; --dangerously-bypass-approvals-and-sandbox runs headless without prompts.
// The flag surface is confirmed against codex-cli when the Codex cassette is
// recorded (human-gated); käsi never runs an agent loop of its own.
func (c *Codex) startProcess(dir, transcriptRel, session string, resume, last bool, env map[string]string) (*exec.Cmd, io.ReadCloser, *os.File, error) {
	transcript, err := os.Create(filepath.Join(dir, transcriptRel))
	if err != nil {
		return nil, nil, nil, err
	}
	prompt := codexPrompt
	var args []string
	if resume {
		// A resumed session carries the prior turn's history; lead with the new-turn
		// instruction so the agent acts on the new message instead of re-affirming
		// completion and writing no reply (decision-019).
		prompt = codexPreamble + codexPrompt
		args = []string{
			"exec", "resume",
			"--json",
			"--skip-git-repo-check",
			"--dangerously-bypass-approvals-and-sandbox",
		}
		if last {
			args = append(args, "--last") // continue the newest session in cmd.Dir
		} else {
			args = append(args, session) // the recorded thread id, a positional
		}
	} else {
		args = []string{
			"exec",
			"--json",
			"--skip-git-repo-check",
			"--dangerously-bypass-approvals-and-sandbox",
			"-C", dir,
		}
	}
	args = append(args, prompt) // the prompt is the trailing positional

	cmd := exec.Command(c.bin, args...) // not CommandContext: Wait/Signal own the lifetime
	cmd.Dir = dir                       // resume has no -C, so cmd.Dir scopes cwd to the task
	if len(env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		transcript.Close()
		return nil, nil, nil, fmt.Errorf("agents: codex stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // its own group, so Signal reaches children
	if err := cmd.Start(); err != nil {
		transcript.Close()
		return nil, nil, nil, fmt.Errorf("agents: codex start: %w", err)
	}
	return cmd, pipe, transcript, nil
}

// Wait blocks until the run exits or ctx is cancelled (a stop or crash), then
// returns the Result — exit code, transcript path, out/ manifest, and whether it
// was stopped (docs/05).
func (c *Codex) Wait(ctx context.Context, h Handle) Result {
	run := c.awaitRun(ctx, h)
	if run == nil {
		return Result{Stopped: true, TranscriptPath: fmt.Sprintf("transcript-%d.jsonl", h.RunID)}
	}
	// Both branches below block until the process has fully exited (run.done), so the
	// deferred finalize runs only once the credential is final: it writes back any
	// token codex rotated this turn and removes the transient CODEX_HOME
	// (decision-025). A no-op when no home was materialized (every twin ring).
	defer c.finalizeCredentials(run)
	select {
	case <-ctx.Done():
		c.signalRun(run)
		err := <-run.done
		return Result{Exit: exitCode(err), TranscriptPath: run.transcriptRel, OutManifest: c.manifest(run), Stopped: true}
	case err := <-run.done:
		return Result{Exit: exitCode(err), TranscriptPath: run.transcriptRel, OutManifest: c.manifest(run), Stopped: false}
	}
}

// awaitRun returns the live run matching the handle, blocking until Start registers
// it if Wait raced ahead — or nil if ctx is cancelled first.
func (c *Codex) awaitRun(ctx context.Context, h Handle) *codexRun {
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			c.mu.Lock()
			c.cond.Broadcast()
			c.mu.Unlock()
		case <-stop:
		}
	}()

	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		if run := c.runs[h.TaskID]; run != nil && run.runID == h.RunID {
			return run
		}
		if ctx.Err() != nil {
			return nil
		}
		c.cond.Wait()
	}
}

// IsLive reports whether this process has a live run matching the handle — false
// after a restart wiped the ephemeral runs map, the signal the agent-watch source
// uses to (re)launch exactly once (decision-015, decision-024).
func (c *Codex) IsLive(h Handle) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	run := c.runs[h.TaskID]
	return run != nil && run.runID == h.RunID
}

// Signal asks the run's process group to terminate — graceful first, hard after a
// short grace period (docs/05).
func (c *Codex) Signal(ctx context.Context, h Handle) error {
	c.mu.Lock()
	run := c.runs[h.TaskID]
	c.mu.Unlock()
	if run == nil {
		return nil
	}
	return c.signalRun(run)
}

func (c *Codex) signalRun(run *codexRun) error {
	if run.cmd.Process == nil {
		return nil
	}
	pgid := run.cmd.Process.Pid
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	go func() {
		time.Sleep(3 * time.Second)
		_ = syscall.Kill(-pgid, syscall.SIGKILL) // no-op if the group is already gone
	}()
	return nil
}

// finalizeCredentials closes out a per-run CODEX_HOME after the turn's process has
// exited (decision-025): it writes back a token codex rotated during the turn and
// removes the transient home so the ~4KB credential never lingers on disk. It runs
// deferred from Wait after run.done, so auth.json is final. When no home was
// materialized (codexHome == "" — every twin ring, and any harness but the real
// Codex) it returns immediately, keeping the twin rings' behaviour and their logs
// and cassettes untouched.
func (c *Codex) finalizeCredentials(run *codexRun) {
	if run.codexHome == "" {
		return
	}
	defer os.RemoveAll(run.codexHome)
	if c.refresh == nil {
		return // no real credential store to refresh (live-capture ring)
	}
	now, err := os.ReadFile(filepath.Join(run.codexHome, "auth.json"))
	if err != nil {
		return // codex left no credential to persist
	}
	if run.authFound && bytes.Equal(now, run.authBefore) {
		return // unchanged this turn — no rotation to write back
	}
	// codex rotated the token mid-turn: persist it back to the reserved reference at
	// the edge (decision-004) — a direct Set, never a message, so no per-run log entry
	// is emitted and the replay/cassette rings stay byte-identical.
	if err := c.refresh.Set(codexAuthRef, string(now)); err != nil {
		log.Printf("agents: codex refresh credential: %v", err)
	}
}

// materializeCodexHome builds a PRIVATE, transient CODEX_HOME for one real Codex
// turn (decision-025, the linchpin). It lives in the OS temp dir — OUTSIDE the task
// workspace and out/, so the harvest and archive never touch it (decision-004) — and
// carries a config.toml and the operator's credential as a 0600 auth.json. Only the
// returned DIR PATH is meant to ride the run env; the ~4KB blob itself NEVER becomes
// an OS env var, which would leak it via /proc/<pid>/environ. Called only when the
// resolved harness is the real Codex, so it is inert in every twin ring.
func materializeCodexHome(ctx context.Context, e Edges) (string, error) {
	home, err := os.MkdirTemp("", "kasi-codex-home-")
	if err != nil {
		return "", fmt.Errorf("agents: codex home: %w", err)
	}
	if err := seedCodexConfig(home); err != nil {
		os.RemoveAll(home)
		return "", err
	}
	if err := seedCodexAuth(ctx, e, home); err != nil {
		os.RemoveAll(home)
		return "", err
	}
	return home, nil
}

// seedCodexConfig writes config.toml into a per-run home: the host's own
// ~/.codex/config.toml when present (so a run inherits the operator's settings),
// else a minimal placeholder — codex falls back to its built-in defaults without
// one. 0600 because the home also holds the credential.
func seedCodexConfig(home string) error {
	dst := filepath.Join(home, "config.toml")
	if data, err := os.ReadFile(hostCodexFile("config.toml")); err == nil {
		return os.WriteFile(dst, data, 0o600)
	}
	return os.WriteFile(dst, []byte("# käsi-managed per-run Codex home\n"), 0o600)
}

// seedCodexAuth writes the operator's credential into a per-run home as a 0600
// auth.json, materialized at the edge (decision-004). It prefers the reserved
// secret — the /codex sign-in stores the credential there — and Resolve errors when
// it is absent (the un-signed-in box), so it falls back to copying the host's
// ~/.codex/auth.json, preserving today's host-logged-in posture. When neither
// exists it leaves auth.json unwritten: codex then fails to authenticate and the run
// records the error, rather than käsi inventing a credential.
func seedCodexAuth(ctx context.Context, e Edges, home string) error {
	dst := filepath.Join(home, "auth.json")
	if blob, err := e.Secrets.Resolve(ctx, codexAuthRef); err == nil {
		return os.WriteFile(dst, []byte(blob), 0o600)
	}
	if data, err := os.ReadFile(hostCodexFile("auth.json")); err == nil {
		return os.WriteFile(dst, data, 0o600)
	}
	return nil
}

// hostCodexFile resolves a file under the host operator's ~/.codex directory. On a
// box with no resolvable home it returns "", so the caller's ReadFile misses and
// falls back to its default — never a panic.
func hostCodexFile(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex", name)
}

// manifest lists out/ RECURSIVELY as sorted paths relative to out/, files only —
// the same whole-tree crossing the seam as the Claude adapter (decision-011).
func (c *Codex) manifest(run *codexRun) []string {
	var names []string
	err := filepath.WalkDir(run.outDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(run.outDir, p)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil
	}
	sort.Strings(names)
	return names
}
