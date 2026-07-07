package msg

import "github.com/dhamidi/k-si/runtime"

// "remember" — upsert a memory by name from its RAW file; the reducer derives the description (store raw, derive on replay)
const Remember = "remember"

type RememberPayload struct {
	Name    string `json:"name"`
	Content []byte `json:"content"`
}

func NewRemember(p RememberPayload) runtime.Msg {
	return runtime.NewMsg(Remember, p)
}
