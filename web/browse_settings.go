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
	tasksPath, _ := s.router.Path("tasks.index", nil)
	view := SettingsView{
		Settings:  s.settingRows(s.app.View(), s.settingShowPath),
		TasksPath: tasksPath,
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
	s.writeSetting(w, r, http.StatusOK, setting, form.Fields)
}

// writeSetting renders view_setting from a setting and its (already bound) form
// fields — a fresh GET passes the current fields; an invalid submit passes fields
// carrying the user's values and per-field errors (form_setting.go).
func (s *Server) writeSetting(w http.ResponseWriter, r *http.Request, status int, setting settings.Setting, fields []settings.Field) {
	index, _ := s.router.Path("settings.index", nil)
	view := SettingView{
		Key:       setting.Key,
		Short:     setting.Short,
		Long:      setting.Long,
		Fields:    settingFieldViews(fields),
		SavePath:  s.settingSavePath(setting.Key),
		IndexPath: index,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := RenderSetting(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render setting: %v", err)
	}
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
