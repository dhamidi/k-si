package web

import (
	"net/http"

	"github.com/dhamidi/dispatch"

	"github.com/dhamidi/k-si/settings"
)

// saveSetting is the write loop for a setting (docs/16, decision-020): look the
// setting up by key (404 if unknown), build its form, bind the submitted values,
// gate the sensitive fields, then Parse. On FieldErrors it re-renders the form
// (422) with the errors bound onto the fields and the user's values preserved. On
// success it emits the setting's set-* message (App.Send blocks until applied) and
// 303-redirects to the index, which renders the new value.
//
// The decision-004 sensitive-field gate (secret → secrets.Set, file →
// content.AddArchive, both substituted BEFORE Parse) would run here on the flat
// path, exactly as Flow C's answer handler runs it. Today's flat settings carry no
// secret or file field, so the gate is a no-op: every field parses through Form.Parse.
func (s *Server) saveSetting(w http.ResponseWriter, r *http.Request) {
	params, _ := dispatch.ParamsFromContext(r.Context())
	setting, ok := s.settingByKey(params["key"])
	if !ok {
		http.Error(w, "no such setting", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	form := setting.Read(s.app.View()).ToForm()
	bound := form.Bind(submittedValues(r, form.Fields))

	// A render-only form (no Parse) cannot accept a submission; flat settings always
	// carry a Parse from the default former, so this only guards a future misuse.
	if form.Parse == nil {
		http.Error(w, "this setting cannot be edited", http.StatusInternalServerError)
		return
	}

	value, errs := form.Parse(bound)
	if !errs.OK() {
		s.writeSetting(w, r, http.StatusUnprocessableEntity, setting, withErrors(bound.Fields, errs))
		return
	}

	// App.Send blocks until applied, so the redirected GET shows the new value.
	s.app.Send(setting.Write(value))

	index, _ := s.router.Path("settings.index", nil)
	http.Redirect(w, r, index, http.StatusSeeOther)
}

// submittedValues reads the raw string for each leaf field off the request by its
// dotted Name — the "re-seed the shape with what the user typed" map Form.Bind
// consumes (docs/16). It recurses into groups/list items; a flat leaf is a single
// "value" field.
func submittedValues(r *http.Request, fields []settings.Field) map[string]string {
	values := map[string]string{}
	collectSubmitted(r, fields, values)
	return values
}

func collectSubmitted(r *http.Request, fields []settings.Field, into map[string]string) {
	for _, fld := range fields {
		if len(fld.Fields) > 0 {
			collectSubmitted(r, fld.Fields, into)
			continue
		}
		into[fld.Name] = r.FormValue(fld.Name)
	}
}

// withErrors binds each field's parse error back onto the (already value-bound)
// fields for an invalid re-render — the user's values stay, the messages appear
// (docs/16). Recurses into nested fields keyed by dotted path.
func withErrors(fields []settings.Field, errs settings.FieldErrors) []settings.Field {
	out := make([]settings.Field, len(fields))
	for i, fld := range fields {
		fld.Error = errs[fld.Name]
		if len(fld.Fields) > 0 {
			fld.Fields = withErrors(fld.Fields, errs)
		}
		out[i] = fld
	}
	return out
}
