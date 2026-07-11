package agents

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"
)

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
// Unlike Claude, Codex MINTS its own session id rather than accepting a supplied
// one, so Start returns a fresh id (recorded via record-session, decision-024) and
// Resume continues by that id. The CLI must be signed in with the operator's
// ChatGPT subscription in the process environment — käsi resolves no token into the
// model or the log (decision-004); the subscription auth rides the CLI's own
// logged-in credential, exactly as NewClaude assumes `claude` is authenticated.
type Codex struct {
	bin     string
	workdir string

	mu   sync.Mutex
	cond *sync.Cond
	runs map[int64]*codexRun // keyed by taskID
}

type codexRun struct {
	cmd           *exec.Cmd
	runID         int64
	session       string
	transcriptRel string
	outDir        string
	done          chan error
}

var _ Harness = (*Codex)(nil)

// NewCodex builds the Codex harness over a workspace root ($WORKDIR). The `codex`
// CLI must be signed in with the operator's ChatGPT subscription in the process's
// environment (decision-004: the subscription credential is the CLI's own, never a
// token in the model or the log).
func NewCodex(workdir string) *Codex {
	c := &Codex{bin: "codex", workdir: workdir, runs: map[int64]*codexRun{}}
	c.cond = sync.NewCond(&c.mu)
	return c
}

// Start opens a NEW Codex session for a task's first turn (decision-024). Codex
// mints its own session id, so we generate one, hand it to the CLI, and return it
// on the Handle for record-session to persist.
func (c *Codex) Start(ctx context.Context, taskID, runID int64, env map[string]string) (Handle, error) {
	return c.spawn(taskID, runID, mintSession(), false, env)
}

// Resume continues an existing Codex session for a later turn. session is the
// minted id recorded on the run; falling back to sessionFor keeps a resume that
// somehow lost its id (the mint→record-session crash window) from starting blank —
// the rollout-scan recovery fork hardens this further, out of this chunk's scope.
func (c *Codex) Resume(ctx context.Context, taskID, runID int64, session string, env map[string]string) (Handle, error) {
	if session == "" {
		session = sessionFor(taskID)
	}
	return c.spawn(taskID, runID, session, true, env)
}

func (c *Codex) spawn(taskID, runID int64, session string, resume bool, env map[string]string) (Handle, error) {
	dir := filepath.Join(c.workdir, fmt.Sprintf("task-%d", taskID))
	if err := os.MkdirAll(filepath.Join(dir, "out"), 0o755); err != nil {
		return Handle{}, err
	}
	transcriptRel := fmt.Sprintf("transcript-%d.jsonl", runID)

	cmd, pipe, transcript, err := c.startProcess(dir, transcriptRel, session, resume, env)
	if err != nil {
		return Handle{}, err
	}

	run := &codexRun{
		cmd:           cmd,
		runID:         runID,
		session:       session,
		transcriptRel: transcriptRel,
		outDir:        filepath.Join(dir, "out"),
		done:          make(chan error, 1),
	}
	go func() {
		_, _ = io.Copy(transcript, pipe) // streams events to disk as they arrive
		err := cmd.Wait()
		transcript.Close()
		run.done <- err
	}()

	c.mu.Lock()
	c.runs[taskID] = run
	c.cond.Broadcast()
	c.mu.Unlock()
	return Handle{TaskID: taskID, RunID: runID, Session: session}, nil
}

// startProcess builds and starts one codex worker process in dir. Codex records
// its own event stream raw; käsi captures the raw bytes to the transcript for the
// harness-dispatched reader (decision-024, transcript fork) — verbatim as-received,
// like Claude's stream-json. --session picks the session id; resume continues it.
// The exact flag surface is confirmed against the live CLI when the Codex cassette
// is recorded (human-gated); käsi never runs an agent loop of its own.
func (c *Codex) startProcess(dir, transcriptRel, session string, resume bool, env map[string]string) (*exec.Cmd, io.ReadCloser, *os.File, error) {
	transcript, err := os.Create(filepath.Join(dir, transcriptRel))
	if err != nil {
		return nil, nil, nil, err
	}
	args := []string{
		"exec",
		"--json",
		"--cd", dir,
		"--session", session,
	}
	prompt := codexPrompt
	if resume {
		// A resumed session carries the prior turn's history; lead with the new-turn
		// instruction so the agent acts on the new message instead of re-affirming
		// completion and writing no reply (decision-019).
		prompt = codexPreamble + codexPrompt
	}
	args = append(args, prompt) // the prompt is the trailing positional

	cmd := exec.Command(c.bin, args...) // not CommandContext: Wait/Signal own the lifetime
	cmd.Dir = dir
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

// mintSession generates a fresh session id for a new Codex run — a UUIDv4, so the
// value differs from the deterministic sessionFor and start-agent-run records it
// (decision-024). Randomness enters at the edge, never in a pure handler.
func mintSession() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("agents: codex session mint: crypto/rand: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
