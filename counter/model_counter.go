package counter

import "github.com/dhamidi/k-si/runtime"

// Model is the counter slice of the application model (docs/15).
type Model struct {
	Count int64 `json:"count"`
}

// Count is the exported pure read other edges use — the only way anything
// outside this package looks at counter's state (docs/15).
func Count(v runtime.View) int64 {
	return runtime.Slice[Model](v, "counter").Count
}
