package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"
)

// CompletionView is the data view_completion.vue renders — the completion-link confirmation: this task is done.
// htmlc receives map[string]any, and idiomatically every value in it is a
// struct like this one: built from the model by the route handler, never a
// raw model object and never an ad-hoc map (docs/08, docs/15).
type CompletionView struct{}

// RenderCompletion writes the full page (docs/08).
func RenderCompletion(ctx context.Context, w io.Writer, engine *htmlc.Engine, view CompletionView) error {
	return engine.RenderPage(ctx, w, "view_completion", map[string]any{
		"completion": view,
	})
}
