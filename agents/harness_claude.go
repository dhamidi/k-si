package agents

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// workerPrompt is käsi's standing instruction to a worker agent (docs/05) — the
// file-based contract, prepended to the task's own inputs, which the agent reads
// from ./in/.
const workerPrompt = `You are käsi's worker agent, running headless in a task workspace.

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
         exactly like the field (e.g. field "bank-login" → $bank-login). Secrets
         never appear in ./in/, a file, or the message history — read them only
         from the environment.

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
stops and unregisters it. Registering does not change how the app starts, so "run
it to try it" and "let käsi run it" are the same program.

./out/memory/ is that user-visible memory — the durable facts about the user and
their world (a preference, an account detail, a decision) that käsi shows and
curates on its web UI. This is DISTINCT from ./store/: store is your private
scratch; memory is knowledge the user sees. When the user tells you to remember
something, or you learn a fact worth keeping, write ./out/memory/<name>.md — the
file name is the fact's identity, so writing the same name again updates it. The
body is raw markdown, one fact per file; lead it with a short YAML "description:"
between --- fences (as a skill's SKILL.md does) and that line becomes the fact's
summary in the index — without one the fact is still saved, just unlabeled. Every
fact käsi knows is provisioned into each run as ./in/memory/<name>.md files plus a
./in/MEMORY.md index: read them for what it already knows. To FORGET a fact, delete
its ./in/memory/<name>.md. Names are flat slugs (no nested paths); ./in/MEMORY.md
is the index, not a note.

To teach yourself for future runs, you may write a reusable SKILL as an Agent
Skills directory under ./out/skills/<name>/ — a ./out/skills/<name>/SKILL.md with
YAML frontmatter (a "name:" matching the folder <name> and a "description:" of
what the skill does and when to use it) between --- fences, then Markdown
instructions, plus any optional ./out/skills/<name>/scripts/ or references/ files
it needs. It is saved durably and provisioned into ./.claude/skills/<name>/ on
future runs, where you discover it as an Agent Skill automatically. Every skill
käsi has learned is already installed there for you — you don't need to load them
by hand.

Never wait for input — always stop.`

// resumePreamble leads the prompt on a RESUME turn (docs/05). A resumed session
// still holds the prior turn's transcript — which may end in "task complete,
// stopping" — so without this the agent tends to re-affirm it is done and write no
// new reply.txt; the harvest then re-sends the PRIOR reply verbatim (the "same
// email twice" bug, decision-019). This makes the new turn explicit: act on the
// new message and always write a fresh reply. It is prepended to workerPrompt so
// the standing file-contract still travels with every turn.
const resumePreamble = `A NEW message has arrived in this ongoing task. This is a FRESH turn in a
continuing conversation — NOT a review of finished work. Whatever you did on an
earlier turn is already done and its reply was already sent; do not repeat it, and
do not decide the task is complete just because earlier work is finished. Read
./in/body.txt for what the user is asking NOW and do that work. ./out/ has been
emptied for this turn, so you MUST write a fresh ./out/reply.txt (or ./out/
request.json) for anything to be sent back — if you write nothing, the user hears
nothing. Then stop.

--- standing instructions (unchanged every turn) ---

`


// Claude is the default harness adapter (docs/05): it shells out to the Claude
// CLI, running one worker turn per task in the task's workspace. It is the
// on-disk twin of SimHarness — the same interface over a real subprocess. Nothing
// outside this file knows which harness is in use.
type Claude struct {
	bin     string
	workdir string

	mu   sync.Mutex
	cond *sync.Cond
	runs map[int64]*claudeRun // keyed by taskID
}

type claudeRun struct {
	cmd           *exec.Cmd
	runID         int64
	session       string
	transcriptRel string
	outDir        string
	done          chan error
}

var _ Harness = (*Claude)(nil)

// NewClaude builds the Claude harness over a workspace root ($WORKDIR). The
// Claude CLI must be authenticated in the process's environment (docs/06: an
// API key would be resolved into the env here; the CLI's own auth is used when
// present).
func NewClaude(workdir string) *Claude {
	c := &Claude{bin: "claude", workdir: workdir, runs: map[int64]*claudeRun{}}
	c.cond = sync.NewCond(&c.mu)
	return c
}

// Start opens a new session for a task's first turn (docs/05).
func (c *Claude) Start(ctx context.Context, taskID, runID int64, env map[string]string) (Handle, error) {
	return c.spawn(taskID, runID, sessionFor(taskID), false, env)
}

// Resume continues an existing session for a later turn.
func (c *Claude) Resume(ctx context.Context, taskID, runID int64, session string, env map[string]string) (Handle, error) {
	if session == "" {
		session = sessionFor(taskID)
	}
	return c.spawn(taskID, runID, session, true, env)
}

func (c *Claude) spawn(taskID, runID int64, session string, resume bool, env map[string]string) (Handle, error) {
	dir := filepath.Join(c.workdir, fmt.Sprintf("task-%d", taskID))
	// The workspace and in/ are laid in by tasks before the run (docs/05); we
	// only guarantee out/ exists for the agent to write into.
	if err := os.MkdirAll(filepath.Join(dir, "out"), 0o755); err != nil {
		return Handle{}, err
	}
	transcriptRel := fmt.Sprintf("transcript-%d.jsonl", runID)

	cmd, pipe, transcript, stderr, err := c.startProcess(dir, transcriptRel, session, resume, env)
	if err != nil {
		return Handle{}, err
	}

	run := &claudeRun{
		cmd:           cmd,
		runID:         runID,
		session:       session,
		transcriptRel: transcriptRel,
		outDir:        filepath.Join(dir, "out"),
		done:          make(chan error, 1),
	}
	go func() {
		_, _ = io.Copy(transcript, pipe) // streams events to disk as they arrive
		err := cmd.Wait()                // safe: pipe hit EOF, so all reads are done (StdoutPipe ordering)
		transcript.Close()
		// Self-heal a restart orphan (decision-015): a first-turn run relaunched
		// after a crash is started with --session-id, but the dead process already
		// created that session, so claude exits "Session ID … already in use". Retry
		// once as --resume, which continues the existing session. Only for a fresh
		// Start (resume==false); a --resume that fails is a genuine failure, not a
		// conflict, so it is never retried (no loop).
		if err != nil && !resume && sessionInUse(stderr.String()) {
			cmd2, pipe2, transcript2, _, err2 := c.startProcess(dir, transcriptRel, session, true, env)
			if err2 != nil {
				run.done <- err2
				return
			}
			c.mu.Lock()
			run.cmd = cmd2 // so Signal reaches the resumed process
			c.mu.Unlock()
			_, _ = io.Copy(transcript2, pipe2)
			err = cmd2.Wait()
			transcript2.Close()
		}
		run.done <- err
	}()

	c.mu.Lock()
	c.runs[taskID] = run
	c.cond.Broadcast() // wake any Wait that raced ahead of this Start
	c.mu.Unlock()
	return Handle{TaskID: taskID, RunID: runID, Session: session}, nil
}

// startProcess builds and starts one claude worker process in dir, returning its
// stdout pipe (streamed to the transcript as events arrive — a child block-buffers
// a redirected file, so io.Copy's per-chunk writes keep the on-disk stream live for
// a crash or a tailing UI, docs/05), the transcript file, and a buffer capturing
// stderr so spawn can detect a session conflict for the restart-resume self-heal
// (decision-015). --session-id opens a new session; --resume continues an existing
// one. Resolved Flow C secrets (decision-004) enter the worker's environment here,
// at the edge, and nowhere else.
func (c *Claude) startProcess(dir, transcriptRel, session string, resume bool, env map[string]string) (*exec.Cmd, io.ReadCloser, *os.File, *bytes.Buffer, error) {
	transcript, err := os.Create(filepath.Join(dir, transcriptRel))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	args := []string{
		"--print",
		"--output-format", "stream-json", "--verbose",
		"--permission-mode", "bypassPermissions",
		"--add-dir", dir,
	}
	prompt := workerPrompt
	if resume {
		args = append(args, "--resume", session)
		// A resumed session carries the prior turn's "I'm done" history; lead with the
		// new-turn instruction so the agent acts on the new message instead of
		// re-affirming completion and writing no reply (decision-019).
		prompt = resumePreamble + workerPrompt
	} else {
		args = append(args, "--session-id", session)
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
		return nil, nil, nil, nil, fmt.Errorf("agents: claude stdout pipe: %w", err)
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = io.MultiWriter(os.Stderr, stderr)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // its own group, so Signal reaches children
	if err := cmd.Start(); err != nil {
		transcript.Close()
		return nil, nil, nil, nil, fmt.Errorf("agents: claude start: %w", err)
	}
	return cmd, pipe, transcript, stderr, nil
}

// sessionInUse reports whether claude refused --session-id because the session
// already exists — the signal that a restart orphan should be resumed instead of
// re-created (decision-015). Confirmed against the CLI: `--session-id X` on an
// existing X exits "Session ID X is already in use", and `--resume X` then continues.
func sessionInUse(stderr string) bool {
	return strings.Contains(stderr, "already in use")
}

// Wait blocks until the run exits or ctx is cancelled (a stop or crash), then
// returns the Result — exit code, transcript path, out/ manifest, and whether it
// was stopped (docs/05).
func (c *Claude) Wait(ctx context.Context, h Handle) Result {
	// The agent-watch subscription can start (and call Wait) before the
	// start-agent-run effect has registered the run; wait for Start rather than
	// treating that race as a stop.
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
		// A natural exit is not a stop; if this was actually a signalled run, the
		// model (its status is stopping) is the authority and corrects it in the
		// finish-agent-run handler. No mutable "stopped" field to race (docs/01).
		return Result{Exit: exitCode(err), TranscriptPath: run.transcriptRel, OutManifest: c.manifest(run), Stopped: false}
	}
}

// awaitRun returns the live run matching the handle, blocking until Start
// registers it if Wait raced ahead — or nil if ctx is cancelled first (a stop or
// crash before the process even started).
func (c *Claude) awaitRun(ctx context.Context, h Handle) *claudeRun {
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

// IsLive reports whether this process has a live run matching the handle —
// false after a restart wiped the ephemeral runs map, the signal the agent-watch
// source uses to (re)launch exactly once (decision-015).
func (c *Claude) IsLive(h Handle) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	run := c.runs[h.TaskID]
	return run != nil && run.runID == h.RunID
}

// Signal asks the run's process group to terminate — graceful first, hard after
// a short grace period (docs/05).
func (c *Claude) Signal(ctx context.Context, h Handle) error {
	c.mu.Lock()
	run := c.runs[h.TaskID]
	c.mu.Unlock()
	if run == nil {
		return nil
	}
	return c.signalRun(run)
}

func (c *Claude) signalRun(run *claudeRun) error {
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
// ["reply.txt", "skills/pay/SKILL.md", "skills/pay/scripts/run.sh"] — so the whole
// output tree crosses the seam (decision-011).
func (c *Claude) manifest(run *claudeRun) []string {
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

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}
