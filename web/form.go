package web

import "flag"

// FormErrors maps a field name to its error message. Form objects carry one
// so an invalid submission re-renders the same view with values and errors
// intact; templates read it directly: v-if="form.Errors.address" (docs/08).
type FormErrors map[string]string

// Set records the first error for a field; later errors keep the first, so
// validation reads top-to-bottom and reports the primary problem per field.
func (e FormErrors) Set(field, message string) {
	if _, taken := e[field]; !taken {
		e[field] = message
	}
}

// Parse binds a raw submitted string into a rich value through the stdlib
// flag.Value interface — one string-to-value contract for forms and CLI
// flags alike (docs/15). A Set error becomes the field's error message.
func (e FormErrors) Parse(field, raw string, into flag.Value) {
	if err := into.Set(raw); err != nil {
		e.Set(field, err.Error())
	}
}
