package msg

import "github.com/dhamidi/k-si/runtime"

// "set-base-url" — set the public base URL capability links are built against; seeded once from -base-url, UI-owned after (decision-020)
const SetBaseURL = "set-base-url"

type SetBaseURLPayload struct {
	URL string `json:"url"`
}

func NewSetBaseURL(p SetBaseURLPayload) runtime.Msg {
	return runtime.NewMsg(SetBaseURL, p)
}
