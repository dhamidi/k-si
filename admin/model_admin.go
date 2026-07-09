package admin

import (
	"fmt"
	"net/url"

	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/settings"
)

// Model is the admin slice of the application model (docs/15): system-wide,
// ownerless configuration. Today just the public base URL, migrated from a
// boot-frozen edge into a logged, editable value (docs/16, decision-020).
type Model struct {
	BaseURL BaseURL `json:"base_url"`
}

// BaseURL is the public origin capability links are built against (docs/04),
// now a model value rather than an edge. It parses through flag.Value — the same
// one-string-to-value contract the CLI flag uses (docs/15) — and forms through
// the default former (a single text field), so it needs no explicit form shape.
type BaseURL string

// Set validates and stores an absolute URL; a relative or malformed one is
// rejected, so a bad value never reaches a capability link.
func (b *BaseURL) Set(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		// ast-grep-ignore: no-placeholder-domain  illustrative text in a validation hint, never a live base-url
		return fmt.Errorf("must be an absolute URL like https://kasi.example.com")
	}
	*b = BaseURL(raw)
	return nil
}

func (b BaseURL) String() string { return string(b) }

// ToForm gives the base URL its form through the default former — one text field
// (settings.FormOf reflects the string kind and parses back through Set above).
func (b BaseURL) ToForm() settings.Form { return settings.FormOf(&b) }

// BaseURLOf reads the current public origin out of the model — empty until
// set-base-url is first applied (an old log converges to empty trivially). Named
// with the -Of suffix because the type already owns the bare name.
func BaseURLOf(v runtime.View) BaseURL {
	return runtime.Slice[Model](v, "admin").BaseURL
}
