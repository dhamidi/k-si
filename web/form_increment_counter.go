package web

import (
	"net/http"
	"strings"

	"github.com/dhamidi/k-si/counter"
	msg "github.com/dhamidi/k-si/counter/msg"
	"github.com/dhamidi/k-si/runtime"
)

// IncrementCounterForm — move the counter by a positive amount
//
// A form object carries its own values and errors: bound from the request,
// validated, re-rendered by the same view when invalid, and turned into
// exactly one imperative runtime message when valid (docs/08, docs/15). It
// is passed to htmlc as a struct value in the props map, like any View.
//
// Fields are raw strings — what the browser sent — so a re-render always
// echoes exactly what was typed. Rich values parse in Validate through the
// stdlib flag.Value contract (docs/15).
type IncrementCounterForm struct {
	By     string // parsed into counter.Amount by Validate
	Errors FormErrors
}

// BindIncrementCounterForm reads the submitted values. Binding never fails — bad
// input becomes field errors in Validate, not an HTTP error.
func BindIncrementCounterForm(r *http.Request) IncrementCounterForm {
	return IncrementCounterForm{
		By:     strings.TrimSpace(r.FormValue("by")),
		Errors: FormErrors{},
	}
}

// Validate returns the form with any field errors recorded. Amount's Set is
// the whole rule; its error text is the field's message (docs/15).
func (f IncrementCounterForm) Validate() IncrementCounterForm {
	var by counter.Amount
	f.Errors.Parse("by", f.By, &by)
	return f
}

// Valid reports whether Message may be constructed.
func (f IncrementCounterForm) Valid() bool { return len(f.Errors) == 0 }

// Message constructs the one imperative message a valid submission means
// (docs/08). Call only when Valid().
func (f IncrementCounterForm) Message() runtime.Msg {
	var by counter.Amount
	_ = by.Set(f.By) // Valid() guarantees this parses

	return msg.NewIncrementCounter(msg.IncrementCounterPayload{
		By: int64(by),
	})
}
