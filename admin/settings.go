package admin

import (
	"github.com/dhamidi/k-si/admin/msg"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/settings"
)

// Settings is admin's contribution to the settings surface (docs/16,
// decision-020): the public base URL, the one setting that belongs to no domain.
// A pure function — the contribution point main.go concatenates; no registry, no
// init(). The value stays in admin's own model; this is a read plus a write over
// it, not a relocation.
func Settings() []settings.Setting {
	return []settings.Setting{{
		Key:   "base_url",
		Short: "Public base URL",
		Long:  "The web address käsi builds its reply and request links from. Editing it changes the next reply's link.",
		Owner: "admin",
		Read:  func(v runtime.View) settings.Value { return BaseURLOf(v) },
		Write: func(val settings.Value) runtime.Msg {
			return msg.NewSetBaseURL(msg.SetBaseURLPayload{URL: val.(BaseURL).String()})
		},
	}}
}
