package agents

// AgentRun — one harness invocation for a task turn: its status, resumable
// session, transcript location and exit code (docs/05, docs/15).

// AgentRunID identifies an agent run; it is the log offset of the
// spawn-agent-run message that created it, so ids are deterministic (docs/15).
type AgentRunID int64

// AgentRun is one worker-agent invocation within a task's continuous session.
// A task accumulates one AgentRun per turn; they share the session so the agent
// keeps context across turns (docs/05).
type AgentRun struct {
	ID             AgentRunID `json:"id"`
	TaskID         int64      `json:"task_id"`
	Status         string     `json:"status"`
	Session        string     `json:"session"`
	TranscriptPath string     `json:"transcript_path"`
	Exit           int        `json:"exit"`
}

// Run status values (docs/05 lifecycle): a run is "running" while the harness
// executes, becomes "stopping" between a stop signal and the process exit, then
// "finished" on a normal exit or "stopped" when it was signalled to terminate.
const (
	StatusRunning  = "running"
	StatusFinished = "finished"
	StatusStopped  = "stopped"
	StatusStopping = "stopping"
)
