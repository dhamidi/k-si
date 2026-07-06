package msg

import "github.com/dhamidi/k-si/runtime"

// "answer-ui-request" — sent by the web edge when the user submits the form;
// marks the request answered and resumes the task with the collected inputs
// (reference-only: archive ids and secret:// URLs, no plaintext).
const AnswerUIRequest = "answer-ui-request"

type AnswerUIRequestPayload struct {
	TaskID     int64             `json:"task_id"`
	RunID      int64             `json:"run_id"`
	Values     map[string]string `json:"values"`
	FileRefs   map[string]int64  `json:"file_refs"`
	SecretRefs map[string]string `json:"secret_refs"`
}

func NewAnswerUIRequest(p AnswerUIRequestPayload) runtime.Msg {
	return runtime.NewMsg(AnswerUIRequest, p)
}
