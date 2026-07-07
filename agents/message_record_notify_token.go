package agents

import "github.com/dhamidi/k-si/runtime"

// "record-notify-token" — record the per-run notify token on the AgentRun (set by start-agent-run at the edge)
const RecordNotifyToken = "record-notify-token"

type RecordNotifyTokenPayload struct {
	TaskID int64  `json:"task_id"`
	RunID  int64  `json:"run_id"`
	Token  string `json:"token"`
}

func NewRecordNotifyToken(p RecordNotifyTokenPayload) runtime.Msg {
	return runtime.NewMsg(RecordNotifyToken, p)
}

func registerRecordNotifyToken(mod *runtime.Module) {
	runtime.HandleMsg(mod, RecordNotifyToken, handleRecordNotifyToken)
}

// handleRecordNotifyToken records the per-run notify token minted at the
// start-agent-run edge onto its AgentRun, so the /control/notify endpoint has a
// logged value to validate against. Copy-on-write over s.Runs — never mutate the
// shared snapshot. No commands.
func handleRecordNotifyToken(v runtime.View, s Model, p RecordNotifyTokenPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	runs := append([]AgentRun(nil), s.Runs...) // copy-on-write
	if i := s.findRun(AgentRunID(p.RunID)); i >= 0 {
		runs[i].NotifyToken = p.Token
	}
	s.Runs = runs
	return s, nil
}
