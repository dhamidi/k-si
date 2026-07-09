package settings

import "flag"

// Parse binds a raw submitted string into a rich value through the stdlib
// flag.Value interface — the one string-to-value contract forms and CLI flags
// share (docs/15), lifted to the nested FieldErrors keyed by dotted path. A leaf
// parses through its flag.Value; a group or list parses its children and
// assembles the composite. A Set error becomes the field's error message. This is
// the nested generalisation of web.FormErrors.Parse.
func (e FieldErrors) Parse(field, raw string, into flag.Value) {
	if err := into.Set(raw); err != nil {
		e.Set(field, err.Error())
	}
}

// OK reports whether a parse produced a value (no field errors) — the readable
// gate a Parse closure ends on.
func (e FieldErrors) OK() bool { return len(e) == 0 }
