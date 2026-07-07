package web

import (
	"net/http"
	"strings"

	msg "github.com/dhamidi/k-si/memory/msg"
	"github.com/dhamidi/k-si/runtime"
)

// RememberForm — the owner adds or edits a memory: a name and its Markdown body
//
// A form object carries its own values and errors: bound from the request,
// validated, re-rendered by the same view when invalid, and turned into
// exactly one imperative runtime message when valid (docs/08, docs/15). It
// is passed to htmlc as a struct value in the props map, like any View.
//
// Fields are raw strings — what the browser sent — so a re-render always
// echoes exactly what was typed. Rich values parse in Validate through the
// stdlib flag.Value contract (docs/15).
type RememberForm struct {
	Name    string
	Content string
	Errors  FormErrors
}

// BindRememberForm reads the submitted values. Binding never fails — bad
// input becomes field errors in Validate, not an HTTP error.
func BindRememberForm(r *http.Request) RememberForm {
	return RememberForm{
		Name:    strings.TrimSpace(r.FormValue("name")),
		Content: strings.TrimSpace(r.FormValue("content")),
		Errors:  FormErrors{},
	}
}

// Validate returns the form with any field errors recorded — a memory needs a
// name (its identity/slug) and a body (first error per field wins).
func (f RememberForm) Validate() RememberForm {
	if f.Name == "" {
		f.Errors.Set("name", "a name is required")
	}
	if f.Content == "" {
		f.Errors.Set("content", "the memory body is required")
	}
	return f
}

// Valid reports whether Message may be constructed.
func (f RememberForm) Valid() bool { return len(f.Errors) == 0 }

// Message constructs the one imperative message a valid submission means
// (docs/08). The form's Content is the raw Markdown body the owner typed; the
// message carries it as the memory's raw file bytes, from which the reducer
// derives the description on replay (feature-memory.md). Call only when Valid().
func (f RememberForm) Message() runtime.Msg {
	return msg.NewRemember(msg.RememberPayload{
		Name:    f.Name,
		Content: []byte(f.Content),
	})
}
