package agents

import "github.com/dhamidi/k-si/runtime"

// "record-session" — record the resumable session id a harness MINTED for a run
// (decision-024). Emitted from the start-agent-run effect, and ONLY when the
// session the harness returned differs from the deterministic sessionFor(taskID):
// Claude, the sim, and the recorded twin all return sessionFor, so they never emit
// this and their logs stay byte-identical. Only a harness that mints its own
// session id (Codex) logs it, so the next turn's Resume reads the right session
// from the model. Like record-notify-token, it must be a logged message because
// the effect that captures the session is suppressed on replay.
const RecordSession = "record-session"

type RecordSessionPayload struct {
	TaskID  int64  `json:"task_id"`
	RunID   int64  `json:"run_id"`
	Session string `json:"session"`
}

func NewRecordSession(p RecordSessionPayload) runtime.Msg {
	return runtime.NewMsg(RecordSession, p)
}

func registerRecordSession(mod *runtime.Module) {
	runtime.HandleMsg(mod, RecordSession, handleRecordSession)
}

// handleRecordSession records the minted session id onto its AgentRun. Copy-on-write
// over s.Runs — never mutate the shared snapshot. No commands.
func handleRecordSession(v runtime.View, s Model, p RecordSessionPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	runs := append([]AgentRun(nil), s.Runs...) // copy-on-write
	if i := s.findRun(AgentRunID(p.RunID)); i >= 0 {
		runs[i].Session = p.Session
	}
	s.Runs = runs
	return s, nil
}
