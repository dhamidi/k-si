package web

import (
	"context"
	"io"
	"strings"

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
	// CodexPath links to the Codex sign-in surface (decision-025) — an
	// operator-facing sign-in action that the typed-setting engine does not model,
	// so it lives beside the settings list as a link rather than a Setting row.
	// Reverse-routed, never string-built (no-url-string-building).
	CodexPath string
	// Nav is the shared top-level navbar site_nav.vue renders (navView) — the same
	// entries on every page, this one lit. Reverse-routed, never string-built
	// (no-url-string-building).
	Nav NavView
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
// form (docs/16): a flat leaf's form has one field ("value") whose Value is the
// current text; a dynamic list's form has one field per row, joined so the index
// shows the whole value (e.g. every allowlisted address), not just the first.
// Blank rows are skipped. Empty form (no rows) reads as "".
func settingDisplay(val settings.Value) string {
	form := val.ToForm()
	values := make([]string, 0, len(form.Fields))
	for _, f := range form.Fields {
		if f.Value != "" {
			values = append(values, f.Value)
		}
	}
	return strings.Join(values, ", ")
}

// RenderSettings writes the full page (docs/08).
func RenderSettings(ctx context.Context, w io.Writer, engine *htmlc.Engine, view SettingsView) error {
	return engine.RenderPage(ctx, w, "view_settings", map[string]any{
		"settings": view,
	})
}
