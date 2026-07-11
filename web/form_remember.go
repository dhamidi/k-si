package web

import (
	"net/http"
	"strings"

	"github.com/dhamidi/k-si/memory"
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
//
// The name is trimmed (a slug carries no surrounding space). The content is only
// CRLF→LF normalised, NOT trimmed: browsers submit a <textarea> with CRLF line
// endings, and an owner re-saving an agent-authored (LF) note must store the SAME
// raw bytes the agent did — trimming the trailing newline or leaving CRLF in would
// drift the collection between its two interfaces (feature-memory.md: one
// collection, byte-identical through both faces).
func BindRememberForm(r *http.Request) RememberForm {
	return RememberForm{
		Name:    strings.TrimSpace(r.FormValue("name")),
		Content: strings.ReplaceAll(r.FormValue("content"), "\r\n", "\n"),
		Errors:  FormErrors{},
	}
}

// Validate returns the form with any field errors recorded — a memory needs a
// valid slug name (its identity/file name/index token) and a non-empty body (first
// error per field wins).
func (f RememberForm) Validate() RememberForm {
	switch {
	case f.Name == "":
		f.Errors.Set("name", "a name is required")
	case !memory.ValidName(f.Name):
		f.Errors.Set("name", "a name may contain only letters, digits, dashes, dots, or underscores")
	}
	if strings.TrimSpace(f.Content) == "" {
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
