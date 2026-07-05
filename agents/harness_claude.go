package agents

import (
	"context"
	"fmt"
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

If you need more from the user, write the question into ./out/reply.txt and
stop; they will answer by email and you will be resumed. Never wait for input.`

// Claude is the default harness adapter (docs/05): it shells out to the Claude
// CLI, running one worker turn per task in the task's workspace. It is the
// on-disk twin of SimHarness — the same interface over a real subprocess. Nothing
// outside this file knows which harness is in use.
type Claude struct {
	bin     string
	workdir string

	mu   sync.Mutex
	runs map[int64]*claudeRun // keyed by taskID
}

type claudeRun struct {
	cmd           *exec.Cmd
	runID         int64
	session       string
	transcriptRel string
	outDir        string
	done          chan error
	stopped       bool
}

var _ Harness = (*Claude)(nil)

// NewClaude builds the Claude harness over a workspace root ($WORKDIR). The
// Claude CLI must be authenticated in the process's environment (docs/06: an
// API key would be resolved into the env here; the CLI's own auth is used when
// present).
func NewClaude(workdir string) *Claude {
	return &Claude{bin: "claude", workdir: workdir, runs: map[int64]*claudeRun{}}
}

// Start opens a new session for a task's first turn (docs/05).
func (c *Claude) Start(ctx context.Context, taskID, runID int64) (Handle, error) {
	return c.spawn(taskID, runID, sessionFor(taskID), false)
}

// Resume continues an existing session for a later turn.
func (c *Claude) Resume(ctx context.Context, taskID, runID int64, session string) (Handle, error) {
	if session == "" {
		session = sessionFor(taskID)
	}
	return c.spawn(taskID, runID, session, true)
}

func (c *Claude) spawn(taskID, runID int64, session string, resume bool) (Handle, error) {
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
	cmd.Stdout = transcript
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
		err := cmd.Wait()
		transcript.Close()
		run.done <- err
	}()

	c.mu.Lock()
	c.runs[taskID] = run
	c.mu.Unlock()
	return Handle{TaskID: taskID, RunID: runID, Session: session}, nil
}

// Wait blocks until the run exits or ctx is cancelled (a stop or crash), then
// returns the Result — exit code, transcript path, out/ manifest, and whether it
// was stopped (docs/05).
func (c *Claude) Wait(ctx context.Context, h Handle) Result {
	c.mu.Lock()
	run := c.runs[h.TaskID]
	c.mu.Unlock()
	if run == nil {
		return Result{Stopped: true, TranscriptPath: fmt.Sprintf("transcript-%d.jsonl", h.RunID)}
	}

	select {
	case <-ctx.Done():
		c.signalRun(run)
		err := <-run.done
		return Result{Exit: exitCode(err), TranscriptPath: run.transcriptRel, OutManifest: c.manifest(run), Stopped: true}
	case err := <-run.done:
		return Result{Exit: exitCode(err), TranscriptPath: run.transcriptRel, OutManifest: c.manifest(run), Stopped: run.stopped}
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
	run.stopped = true
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

func (c *Claude) manifest(run *claudeRun) []string {
	entries, err := os.ReadDir(run.outDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
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
