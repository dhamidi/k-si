package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"

	"github.com/dhamidi/k-si/settings"
)

// SettingView is the data view_setting.vue renders — one setting's form (docs/16,
// decision-020): its key, short and long descriptions, the form's fields, and the
// POST target. Built by the route handler from the setting's own ToForm (docs/16),
// never a raw model object (docs/08). Host-gated, no token (decision-006). This is
// the plain embedded form — no turbo-frame, no reshape controls (deferred to
// phase 3).
type SettingView struct {
	Key   string
	Short string
	Long  string
	// Fields is one FieldView per form field, looped through field.vue. On a fresh
	// GET each carries the current value; on an invalid submit each carries the
	// user's value and its per-field error.
	Fields []SettingFieldView
	// SavePath is the POST target of the form — reverse-routed settings.save, never
	// string-built (no-url-string-building).
	SavePath string
	// IndexPath is the crumb back to the settings index (GET /settings).
	IndexPath string
}

// SettingFieldView is one control's render data — a settings.Field flattened to plain
// strings for the template. The Kind crosses the boundary as a plain string
// because htmlc's expr compares a defined string type (settings.Kind) unequal to
// a string literal, so a template `field.Kind == 'text'` needs a real string —
// the same reason Flow C's request FieldView.Type is a string (decision-005). A
// secret/file field never carries a value (decision-004); the former already
// leaves those blank, and field.vue never echoes them regardless.
type SettingFieldView struct {
	Name    string
	Label   string
	Kind    string
	Value   string
	Options []string
	Error   string
}

// settingFieldViews flattens a form's fields into render views (plain-string
// Kind). Flat settings are a single leaf; the loop keeps the general shape.
func settingFieldViews(fields []settings.Field) []SettingFieldView {
	views := make([]SettingFieldView, 0, len(fields))
	for _, f := range fields {
		views = append(views, SettingFieldView{
			Name:    f.Name,
			Label:   f.Label,
			Kind:    string(f.Kind),
			Value:   f.Value,
			Options: f.Options,
			Error:   f.Error,
		})
	}
	return views
}

// RenderSetting writes the full page (docs/08). Pages render with RenderPage so
// the full-<html> document composes with its field sub-components.
func RenderSetting(ctx context.Context, w io.Writer, engine *htmlc.Engine, view SettingView) error {
	return engine.RenderPage(ctx, w, "view_setting", map[string]any{
		"setting": view,
	})
}
