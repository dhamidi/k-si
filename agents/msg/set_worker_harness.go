package msg

import "github.com/dhamidi/k-si/runtime"

// "set-worker-harness" — choose which agent harness fresh runs use (decision-024).
// The operator's control, seeded from serve -harness and editable on the settings
// page. It pins only NEW runs; a task already under way keeps the harness its
// first run was pinned to.
const SetWorkerHarness = "set-worker-harness"

type SetWorkerHarnessPayload struct {
	Name string `json:"name"`
}

func NewSetWorkerHarness(p SetWorkerHarnessPayload) runtime.Msg {
	return runtime.NewMsg(SetWorkerHarness, p)
}
