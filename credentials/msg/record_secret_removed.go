package msg

import "github.com/dhamidi/k-si/runtime"

// "record-secret-removed" — log that a secret reference was deleted (name only); emitted after the edge Delete
const RecordSecretRemoved = "record-secret-removed"

type RecordSecretRemovedPayload struct {
	Ref string `json:"ref"`
}

func NewRecordSecretRemoved(p RecordSecretRemovedPayload) runtime.Msg {
	return runtime.NewMsg(RecordSecretRemoved, p)
}
