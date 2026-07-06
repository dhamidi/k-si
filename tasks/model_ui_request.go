package tasks

// UIRequest — a UI request an agent run raised mid-task (Flow C, docs/05). It is
// an event-sourced model entry, durable via its register-ui-request log record —
// there is no ui_request content table (decision-001). Heavy answered content
// lives in archive (FileRefs) and the secrets database (SecretRefs); the model
// carries only references.
type UIRequest struct {
	// RunID is the raising agent run's id (meta.Offset) — the request's identity
	// and the {id} of its capability link (decision-003).
	RunID    int64  `json:"run_id"`
	TaskID   int64  `json:"task_id"`
	Token    string `json:"token"`
	FormSpec []byte `json:"form_spec"`
	Link     string `json:"link"`
	Status   string `json:"status"`
	// Values, FileRefs, SecretRefs are filled in once the request is answered.
	Values     map[string]string `json:"values"`
	FileRefs   map[string]int64  `json:"file_refs"`
	SecretRefs map[string]string `json:"secret_refs"`
}

const (
	// RequestPending — minted, awaiting the user's answer.
	RequestPending = "pending"
	// RequestAnswered — the user submitted; the task resumes.
	RequestAnswered = "answered"
)
