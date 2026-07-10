package msg

import "github.com/dhamidi/k-si/runtime"

// "record-secret-set" — log that a secret reference was set/rotated (name only, never the value); the web /secrets handler emits it after the edge write
const RecordSecretSet = "record-secret-set"

type RecordSecretSetPayload struct {
	Ref string `json:"ref"`
}

func NewRecordSecretSet(p RecordSecretSetPayload) runtime.Msg {
	return runtime.NewMsg(RecordSecretSet, p)
}
