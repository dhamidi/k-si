package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"

	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/settings"
)

// SettingsView is the data view_settings.vue renders — the /settings index
// (docs/16, decision-020): every contributed setting, its short description, its
// current value, and a link to its form. Built by the route handler from the
// wired []settings.Setting plus a live model read, never a raw model object
// (docs/08, docs/15). Host-gated, no token (decision-006) — the same trust model
// as the other browse pages.
type SettingsView struct {
	Settings []SettingRow
	// TasksPath is the cross-nav back to the hub list page — the crumb the other
	// browse pages carry. Reverse-routed, never string-built (no-url-string-building).
	TasksPath string
}

// SettingRow is one setting in the index list: its key, short description, owning
// module, current display value, and the reverse-routed link to its form.
type SettingRow struct {
	Key      string
	Short    string
	Owner    string
	Value    string
	ShowPath string
}

// settingRows builds one row per wired setting, reading each setting's current
// value out of the live model through its own Read + form (docs/16). The display
// string is the leaf form field's current value — the same string the form would
// render into its input — so the index and the form never diverge.
func (s *Server) settingRows(v runtime.View, showPath func(key string) string) []SettingRow {
	rows := make([]SettingRow, 0, len(s.settings))
	for _, setting := range s.settings {
		rows = append(rows, SettingRow{
			Key:      setting.Key,
			Short:    setting.Short,
			Owner:    setting.Owner,
			Value:    settingDisplay(setting.Read(v)),
			ShowPath: showPath(setting.Key),
		})
	}
	return rows
}

// settingDisplay renders a setting value's current display string through its own
// form: a flat leaf's form has one field ("value") whose Value is the current
// text (docs/16). Empty form defends against a value with no leaf field.
func settingDisplay(val settings.Value) string {
	form := val.ToForm()
	if len(form.Fields) == 0 {
		return ""
	}
	return form.Fields[0].Value
}

// RenderSettings writes the full page (docs/08).
func RenderSettings(ctx context.Context, w io.Writer, engine *htmlc.Engine, view SettingsView) error {
	return engine.RenderPage(ctx, w, "view_settings", map[string]any{
		"settings": view,
	})
}
