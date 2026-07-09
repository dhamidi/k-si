package web

import (
	"context"
	"html"
	"io"
	"log"
	"net/http"

	"github.com/dhamidi/htmlc"

	"github.com/dhamidi/k-si/settings"
)

// SettingView is the data view_setting.vue renders — one setting's form (docs/16,
// decision-020): its key, short and long descriptions, the form's fields, and the
// POST targets. Built by the route handler from the setting's own ToForm (docs/16),
// never a raw model object (docs/08). Host-gated, no token (decision-006).
//
// A DYNAMIC setting (its ToForm carries an Update — the initiator allowlist) sets
// Dynamic true, which turns on the per-row Remove and the list-level Add controls;
// a flat setting leaves it false and renders exactly one Save. The form is wrapped
// in a <turbo-frame id="setting-{key}"> so Turbo can swap it on a reshape; the
// frame HTML is built once and either embedded in the full page (v-html) or
// written alone as the fragment.
type SettingView struct {
	Key   string
	Short string
	Long  string
	// Fields is one FieldView per form field, looped through field.vue. On a fresh
	// GET each carries the current value; on an invalid submit or a reshape each
	// carries the user's value (and, when invalid, its per-field error).
	Fields []SettingFieldView
	// Dynamic is true when the setting's form can reshape (ToForm carries an
	// Update) — it gates the Add/Remove reshape controls.
	Dynamic bool
	// SavePath is the final-submit target — reverse-routed settings.save.
	SavePath string
	// ReshapePath is the add/remove target — reverse-routed settings.reshape. Empty
	// string on a flat setting (no reshape).
	ReshapePath string
	// IndexPath is the crumb back to the settings index (GET /settings).
	IndexPath string
	// FrameID is the <turbo-frame> id ("setting-{key}") Turbo targets on a swap.
	FrameID string
	// FrameHTML is the pre-rendered <turbo-frame>…form…</turbo-frame>, injected into
	// the page via v-html. htmlc cannot emit a hyphenated custom-element tag itself
	// (it reads <turbo-frame> as a component reference), so the frame wrapper is
	// applied around the htmlc-rendered form here at the render edge (docs/16).
	FrameHTML string
	// TurboSrc is the reverse-routed assets.turbo URL, passed to base_styles so it
	// emits the one <script> include on this page.
	TurboSrc string
}

// SettingFieldView is one control's render data — a settings.Field flattened to plain
// strings for the template. The Kind crosses the boundary as a plain string
// because htmlc's expr compares a defined string type (settings.Kind) unequal to
// a string literal, so a template `field.Kind == 'text'` needs a real string —
// the same reason Flow C's request FieldView.Type is a string (decision-005). A
// secret/file field never carries a value (decision-004); the former already
// leaves those blank, and field.vue never echoes them regardless. Index is the
// row's position in the current field set — the Remove button's target index a
// reshape folds through Form.Update.
type SettingFieldView struct {
	Name    string
	Label   string
	Kind    string
	Value   string
	Options []string
	Error   string
	Index   int
}

// settingFieldViews flattens a form's fields into render views (plain-string
// Kind), tagging each with its position so a Remove control can name it.
func settingFieldViews(fields []settings.Field) []SettingFieldView {
	views := make([]SettingFieldView, 0, len(fields))
	for i, f := range fields {
		views = append(views, SettingFieldView{
			Name:    f.Name,
			Label:   f.Label,
			Kind:    string(f.Kind),
			Value:   f.Value,
			Options: f.Options,
			Error:   f.Error,
			Index:   i,
		})
	}
	return views
}

// settingView assembles the render data for one setting from its (already bound)
// form fields. dynamic comes from the setting's form (Update != nil) — the handler
// reads it once and threads it here.
func (s *Server) settingView(setting settings.Setting, dynamic bool, fields []settings.Field) SettingView {
	index, _ := s.router.Path("settings.index", nil)
	reshape := ""
	if dynamic {
		reshape = s.settingReshapePath(setting.Key)
	}
	return SettingView{
		Key:         setting.Key,
		Short:       setting.Short,
		Long:        setting.Long,
		Fields:      settingFieldViews(fields),
		Dynamic:     dynamic,
		SavePath:    s.settingSavePath(setting.Key),
		ReshapePath: reshape,
		IndexPath:   index,
		FrameID:     "setting-" + setting.Key,
		TurboSrc:    s.turboSrc(),
	}
}

// frameHTML renders the setting_form component to a string and wraps it in its
// <turbo-frame> — the one component, rendered once, that both the full page and
// the bare fragment carry (docs/16). The frame id is HTML-escaped defensively,
// though a setting key is always a snake_case slug.
func (s *Server) frameHTML(ctx context.Context, view SettingView) (string, error) {
	form, err := s.engine.RenderFragmentString(ctx, "setting_form", map[string]any{"setting": view})
	if err != nil {
		return "", err
	}
	return `<turbo-frame id="` + html.EscapeString(view.FrameID) + `">` + form + `</turbo-frame>`, nil
}

// renderSettingPage writes the full <html> page (docs/08): the frame-wrapped form
// is built first and embedded via v-html, so the page and the reshape fragment
// share the one setting_form template.
func (s *Server) renderSettingPage(w http.ResponseWriter, r *http.Request, status int, view SettingView) {
	fh, err := s.frameHTML(r.Context(), view)
	if err != nil {
		log.Printf("web: render setting frame: %v", err)
		http.Error(w, "could not render the setting", http.StatusInternalServerError)
		return
	}
	view.FrameHTML = fh

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := RenderSetting(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render setting: %v", err)
	}
}

// renderSettingFragment writes the bare <turbo-frame> fragment Turbo swaps in
// place (docs/16) — the same form component, no page chrome. Content-negotiated
// against the full page on the Turbo-Frame request header.
func (s *Server) renderSettingFragment(w http.ResponseWriter, r *http.Request, status int, view SettingView) {
	fh, err := s.frameHTML(r.Context(), view)
	if err != nil {
		log.Printf("web: render setting fragment: %v", err)
		http.Error(w, "could not render the setting", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, fh); err != nil {
		log.Printf("web: write setting fragment: %v", err)
	}
}

// RenderSetting writes the full page (docs/08). Pages render with RenderPage so
// the full-<html> document composes with its field sub-components.
func RenderSetting(ctx context.Context, w io.Writer, engine *htmlc.Engine, view SettingView) error {
	return engine.RenderPage(ctx, w, "view_setting", map[string]any{
		"setting": view,
	})
}
