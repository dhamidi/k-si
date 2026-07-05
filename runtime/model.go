package runtime

// View is read-only access to the whole model — one value, handed to every
// handler and subscription provider. A domain reads anything through it but
// writes only its own slice, which its handlers receive and return directly
// (docs/01, docs/15).
type View struct {
	slices map[string]any
}

// Slice returns a module's model slice by name. Domain packages wrap this in
// typed, exported read helpers (`tasks.ByThreadKey(v, key)`); nothing outside
// the owning domain casts its slice.
func (v View) Slice(module string) (any, bool) {
	s, ok := v.slices[module]
	return s, ok
}

// Slice is the typed accessor domains use inside their own read helpers.
func Slice[S any](v View, module string) S {
	s, _ := v.slices[module].(S)
	return s
}
