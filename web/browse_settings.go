package web

import (
	"log"
	"net/http"

	"github.com/dhamidi/dispatch"

	"github.com/dhamidi/k-si/settings"
)

// showSettings renders the /settings index (docs/16, decision-020): every wired
// setting with its short description, current value, and a link to its form.
// Host-gated, no token (decision-006). The values come from each setting's own
// Read against the live model (docs/15).
func (s *Server) showSettings(w http.ResponseWriter, r *http.Request) {
	view := SettingsView{
		Settings:  s.settingRows(s.app.View(), s.settingShowPath),
		CodexPath: s.codexIndexPath(),
		Nav:       s.navView("settings.index"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := RenderSettings(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render settings: %v", err)
	}
}

// showSetting renders one setting's form (docs/16): its help text and a control
// per form field, built from the setting's own ToForm against the live model. An
// unknown key is a 404 — the index only ever links wired keys, so this guards a
// hand-typed URL.
func (s *Server) showSetting(w http.ResponseWriter, r *http.Request) {
	params, _ := dispatch.ParamsFromContext(r.Context())
	setting, ok := s.settingByKey(params["key"])
	if !ok {
		http.Error(w, "no such setting", http.StatusNotFound)
		return
	}

	form := setting.Read(s.app.View()).ToForm()
	s.writeSetting(w, r, http.StatusOK, setting, form.Update != nil, form.Fields)
}

// writeSetting renders view_setting (the full page) from a setting and its
// (already bound) form fields — a fresh GET passes the current fields; an invalid
// submit passes fields carrying the user's values and per-field errors
// (form_setting.go). dynamic (the form's Update != nil) turns on the reshape
// controls.
func (s *Server) writeSetting(w http.ResponseWriter, r *http.Request, status int, setting settings.Setting, dynamic bool, fields []settings.Field) {
	s.renderSettingPage(w, r, status, s.settingView(setting, dynamic, fields))
}

// settingByKey finds a wired setting by its key — the small lookup the show and
// save handlers share. No registry: it walks the slice assembled in the open.
func (s *Server) settingByKey(key string) (settings.Setting, bool) {
	for _, setting := range s.settings {
		if setting.Key == key {
			return setting, true
		}
	}
	return settings.Setting{}, false
}

// settingShowPath reverse-routes one setting's form URL (rule
// no-url-string-building).
func (s *Server) settingShowPath(key string) string {
	p, _ := s.router.Path("settings.show", dispatch.Params{"key": key})
	return p
}

// settingSavePath reverse-routes one setting's submit URL.
func (s *Server) settingSavePath(key string) string {
	p, _ := s.router.Path("settings.save", dispatch.Params{"key": key})
	return p
}

// settingReshapePath reverse-routes one setting's add/remove URL — the reshape
// round-trip target (rule no-url-string-building).
func (s *Server) settingReshapePath(key string) string {
	p, _ := s.router.Path("settings.reshape", dispatch.Params{"key": key})
	return p
}

// turboSrc reverse-routes the Turbo asset URL, passed to base_styles so the
// settings pages emit the one <script> include (docs/16).
func (s *Server) turboSrc() string {
	p, _ := s.router.Path("assets.turbo", nil)
	return p
}
