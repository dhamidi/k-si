package agents

import "context"

// Handle identifies a live worker-agent run to the harness edge: the task it
// serves, the run (turn) id, and the resumable session (docs/05).
type Handle struct {
	TaskID  int64  `json:"task_id"`
	RunID   int64  `json:"run_id"`
	Session string `json:"session"`
}

// Result is what a completed (or stopped) run leaves behind: the process exit
// code, where the transcript was written, the manifest of files dropped into
// out/, and whether the run was signalled to stop rather than exiting normally
// (docs/05).
type Result struct {
	Exit           int      `json:"exit"`
	TranscriptPath string   `json:"transcript_path"`
	OutManifest    []string `json:"out_manifest"`
	Stopped        bool     `json:"stopped"`
}

// Harness is the agent-execution edge (docs/05): käsi does not implement an
// agent loop, it shells out to an official harness (Claude by default). Start
// opens a new session, Resume continues an existing one, Wait blocks until the
// current turn completes (or ctx is cancelled by a stop/crash), and Signal asks
// the process to terminate. The real twin drives a subprocess; the sim twin
// (harness_sim.go) rendezvous-delivers scripted turns.
type Harness interface {
	// Start spawns a new session for a task's first turn and returns immediately
	// with a Handle; the work runs in the worker process.
	Start(ctx context.Context, taskID, runID int64) (Handle, error)
	// Resume continues an existing session for a subsequent turn.
	Resume(ctx context.Context, taskID, runID int64, session string) (Handle, error)
	// Wait blocks until the run's turn completes or ctx is cancelled, then
	// returns the Result. On cancellation it returns Result{Stopped:true}.
	Wait(ctx context.Context, h Handle) Result
	// Signal asks the harness process to stop (graceful, then hard).
	Signal(ctx context.Context, h Handle) error
}
