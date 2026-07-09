package msg

import "github.com/dhamidi/k-si/runtime"

// "mint-ui-request" — sent by tasks/agent-run-finished; email mints the token and
// builds the capability link, then emits register-ui-request back to tasks.
const MintUIRequest = "mint-ui-request"

type MintUIRequestPayload struct {
	TaskID int64 `json:"task_id"`
	RunID  int64 `json:"run_id"`
	// BaseURL is the public origin the request link is built against, threaded from
	// admin.Model by the emitting handler — the migrated, editable base URL (docs/16,
	// decision-020). Effects never read the model, so the handler supplies it.
	BaseURL string `json:"base_url"`
}

func NewMintUIRequest(p MintUIRequestPayload) runtime.Cmd {
	return runtime.NewCmd(MintUIRequest, p)
}
