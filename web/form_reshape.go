package web

import (
	"net/http"
	"strconv"

	"github.com/dhamidi/dispatch"

	"github.com/dhamidi/k-si/settings"
)

// reshapeSetting folds one add/remove Event through a dynamic setting's form and
// re-renders it (docs/16, decision-020) — the progressive-enhancement round-trip
// behind the initiator allowlist's Add and Remove buttons. There is NO server
// session: the whole form is rebuilt from the request each time.
//
// The order is bind → update → re-bind, and it matters:
//
//  1. Build the setting's form from the model, then GROW it to the rows the
//     browser submitted (the shape lives in the body, not the model — docs/16).
//  2. Bind the submitted values onto that current shape, so every typed value is
//     captured.
//  3. Fold the Event through Form.Update to change the shape (an added row is
//     empty; a removed row drops out, its name-stable neighbours untouched).
//  4. Re-bind the same submitted values onto the new shape, so nothing typed is
//     lost across the reshape.
//
// It then content-negotiates on the Turbo-Frame request header: present → the
// bare <turbo-frame> fragment Turbo swaps in place; absent → the whole page with
// the new shape and the POSTed values re-rendered (a plain full reload, nothing
// lost without JavaScript).
func (s *Server) reshapeSetting(w http.ResponseWriter, r *http.Request) {
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
	if form.Update == nil {
		// A flat setting has no shape to change: re-render it as-is (the graceful
		// outcome — a stray reshape POST just shows the form again).
		s.writeSetting(w, r, http.StatusOK, setting, false, form.Fields)
		return
	}

	form = growToSubmitted(form, r)
	submitted := submittedValues(r, form.Fields)
	bound := form.Bind(submitted)
	reshaped := bound.Update(bound, reshapeEvent(r))
	reshaped = reshaped.Bind(submitted)

	view := s.settingView(setting, true, reshaped.Fields)
	if r.Header.Get("Turbo-Frame") != "" {
		s.renderSettingFragment(w, r, http.StatusOK, view)
		return
	}
	s.renderSettingPage(w, r, http.StatusOK, view)
}

// reshapeEvent reads the shape-changing Event off the POST. A real-browser submit
// button carries the op in its name (a submit button posts one name=value pair):
// `remove=<index>` or `add=<anything>`. A scenario or scripted client can instead
// send explicit `op`/`index` fields. Both resolve to the same Event.
func reshapeEvent(r *http.Request) settings.Event {
	switch {
	case r.Form.Has("remove"):
		i, _ := strconv.Atoi(r.FormValue("remove"))
		return settings.Event{Op: "remove", Index: i}
	case r.Form.Has("add"):
		return settings.Event{Op: "add"}
	default:
		i, _ := strconv.Atoi(r.FormValue("index"))
		return settings.Event{Op: r.FormValue("op"), Index: i}
	}
}

// growToSubmitted rebuilds a dynamic form's CURRENT shape from the request, since
// the server holds no session and the number of list rows rides the body (docs/16).
// It repeatedly applies Update{add} while every leaf field the add would create is
// present in the POST — so a form grown to N rows in the browser is reconstructed
// to N rows here before the reshape/parse runs. A flat form (Update == nil) is
// returned unchanged. The iteration cap defends against a pathological Update that
// never adds a namable leaf.
func growToSubmitted(form settings.Form, r *http.Request) settings.Form {
	for i := 0; form.Update != nil && i < 1024; i++ {
		grown := form.Update(form, settings.Event{Op: "add"})
		added := newLeafNames(form.Fields, grown.Fields)
		if len(added) == 0 {
			break
		}
		allPresent := true
		for _, name := range added {
			if !r.Form.Has(name) {
				allPresent = false
				break
			}
		}
		if !allPresent {
			break
		}
		form = grown
	}
	return form
}

// leafNames collects every leaf field's dotted Name (recursing into groups/list
// items) — the keys the submitted-values map is keyed by.
func leafNames(fields []settings.Field) []string {
	var out []string
	for _, f := range fields {
		if len(f.Fields) > 0 {
			out = append(out, leafNames(f.Fields)...)
			continue
		}
		out = append(out, f.Name)
	}
	return out
}

// newLeafNames returns the leaf names present in grown but not in old — the
// field(s) an Update{add} introduced.
func newLeafNames(old, grown []settings.Field) []string {
	have := map[string]bool{}
	for _, name := range leafNames(old) {
		have[name] = true
	}
	var out []string
	for _, name := range leafNames(grown) {
		if !have[name] {
			out = append(out, name)
		}
	}
	return out
}
