package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"
)

// CounterView is the data view_counter.vue renders — the canary page: current count and the increment form.
// htmlc receives map[string]any, and idiomatically every value in it is a
// struct like this one: built from the model by the route handler, never a
// raw model object and never an ad-hoc map (docs/08, docs/15).
type CounterView struct {
	Count         int64
	Form          IncrementCounterForm // a form object is a View struct with a memory of what went wrong (docs/15)
	IncrementPath string               // generated from the named route, never hand-built (docs/08)
}

// RenderCounter writes the full page (docs/08).
func RenderCounter(ctx context.Context, w io.Writer, engine *htmlc.Engine, view CounterView) error {
	return engine.RenderPage(ctx, w, "view_counter", map[string]any{
		"counter": view,
	})
}
