package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"
)

// RequestView is the data view_request.vue renders — a UI request an agent
// raised mid-task (Flow C, decision-005): why input is needed, the fields to
// fill, and, once answered, the closed state. Built from the model + the parsed
// form spec by the route handler, never a raw model object (docs/08, docs/15).
type RequestView struct {
	// Message is the agent's ask — the summary that leads the page.
	Message string
	// Fields is one FieldView per spec entry, in spec order.
	Fields []FieldView
	// Action is the POST target (the request link with its token) the form
	// submits to; it is also where an invalid submit re-renders.
	Action string
	// Answered is true once the request is closed; the page then shows the
	// answered state instead of the form.
	Answered bool
}

// FieldView is one control the form renders (decision-005): label + a
// type-appropriate control + any validation error. Value echoes what was
// submitted so an invalid re-render shows the user's own input — except for a
// secret, whose plaintext is never echoed back into the page.
type FieldView struct {
	Name     string
	Label    string
	Type     string
	Required bool
	Options  []string
	Value    string
	Error    string
}

// RenderRequest writes the full page (docs/08). Pages are rendered with
// RenderPage so the .vue full-<html> document composes with its sub-components.
func RenderRequest(ctx context.Context, w io.Writer, engine *htmlc.Engine, view RequestView) error {
	return engine.RenderPage(ctx, w, "view_request", map[string]any{
		"request": view,
	})
}
