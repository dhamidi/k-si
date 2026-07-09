package admin

import (
	"github.com/dhamidi/k-si/admin/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "set-base-url" — set the public base URL capability links are built against; seeded once from -base-url, UI-owned after (decision-020)

func registerSetBaseURL(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.SetBaseURL, handleSetBaseURL)
}

func handleSetBaseURL(v runtime.View, s Model, p msg.SetBaseURLPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// Whole-value replace, the only writer of admin.Model.BaseURL. The value was
	// already validated as an absolute URL at the edge (the flag guard or the
	// settings form's Parse); the reducer just records it. Copy-on-write is trivial
	// here — the model is a single value — but stays the pattern.
	s.BaseURL = BaseURL(p.URL)
	return s, nil
}
