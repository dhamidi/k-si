package email

import "github.com/dhamidi/k-si/runtime"

// "record-poll-state" — records the JMAP inbox high-water state so a restart resumes from it (offline-gap fix, decision-018)
const RecordPollState = "record-poll-state"

type RecordPollStatePayload struct {
	State string `json:"state"`
}

func NewRecordPollState(p RecordPollStatePayload) runtime.Msg {
	return runtime.NewMsg(RecordPollState, p)
}

func registerRecordPollState(mod *runtime.Module) {
	runtime.HandleMsg(mod, RecordPollState, handleRecordPollState)
}

func handleRecordPollState(v runtime.View, s Model, p RecordPollStatePayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// The poll edge advances the high-water mark through this message, so every
	// step of the inbox cursor is an entry in the log — auditable, and rebuilt by
	// replay on restart so the poller resumes exactly where it left off rather than
	// re-anchoring to "now" and skipping mail that arrived while käsi was down
	// (offline-gap fix, decision-018). Recording the effect's result as a value keeps
	// the handler pure; the JMAP Fetch itself runs only live (docs/13).
	s.PollCursor = p.State
	return s, nil
}
