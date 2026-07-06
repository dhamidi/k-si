package agents

import (
	"context"
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

// workerPrompt is käsi's standing instruction to a worker agent (docs/05) — the
// file-based contract, prepended to the task's own inputs, which the agent reads
// from ./in/.
const workerPrompt = `You are käsi's worker agent, running headless in a task workspace.

Your inputs are in ./in/ — read ./in/body.txt (the message to act on) and any
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
collect one without it landing in an email. Never wait for input — always stop.`

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
	transcript, err := os.Create(filepath.Join(dir, transcriptRel))
	if err != nil {
		return Handle{}, err
	}

	args := []string{
		"--print",
		"--output-format", "stream-json", "--verbose",
		"--permission-mode", "bypassPermissions",
		"--add-dir", dir,
	}
	if resume {
		args = append(args, "--resume", session)
	} else {
		args = append(args, "--session-id", session)
	}
	args = append(args, workerPrompt) // the prompt is the trailing positional

	cmd := exec.Command(c.bin, args...) // not CommandContext: Wait/Signal own the lifetime
	cmd.Dir = dir
	// Resolved Flow C secrets (decision-004) enter the worker's environment here,
	// at the edge, and nowhere else. Inherit the parent env, then add them.
	if len(env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	// Stream stdout through a pipe we copy to the file, rather than pointing
	// cmd.Stdout straight at the *os.File. A child's stdio block-buffers a
	// redirected file, so the transcript would only update in chunks (or at
	// exit). io.Copy issues a real write syscall per chunk it reads, so the
	// file grows as events are generated — a crash keeps what was produced, and
	// a future web UI can tail the in-progress stream (docs/05).
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		transcript.Close()
		return Handle{}, fmt.Errorf("agents: claude stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // its own group, so Signal reaches children

	if err := cmd.Start(); err != nil {
		transcript.Close()
		return Handle{}, fmt.Errorf("agents: claude start: %w", err)
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
		run.done <- err
	}()

	c.mu.Lock()
	c.runs[taskID] = run
	c.cond.Broadcast() // wake any Wait that raced ahead of this Start
	c.mu.Unlock()
	return Handle{TaskID: taskID, RunID: runID, Session: session}, nil
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
