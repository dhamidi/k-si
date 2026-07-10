package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"
)

// StoreView is the data view_store.vue renders — the agent's persistent store,
// browsed as a directory tree (docs/08, Flow F decision-012). A view has one of
// two shapes, chosen by IsFile: a directory listing (Entries) or a single file
// (File). Both carry the breadcrumb trail (Crumbs) back up the tree. Built by
// the route handler from käsi's READ-ONLY fs.FS view of the store, never a raw
// model object (docs/08). Host-gated, no token (decision-006); a view never
// writes.
type StoreView struct {
	// Path is the current directory or file path within the store — "" at the
	// root — shown as the heading subtitle.
	Path string
	// Crumbs are the breadcrumb segments from the store root down to Path, each a
	// reverse-routed link (never string-built; rule no-url-string-building).
	Crumbs []StoreCrumb
	// IsFile picks the shape: false renders Entries (a directory), true renders
	// File (one file).
	IsFile bool
	// Entries are a directory's children — dirs first, then files, each sorted —
	// present when IsFile is false.
	Entries []StoreEntry
	// Empty reports an empty directory so the view shows a "nothing here yet"
	// state instead of an empty list.
	Empty bool
	// File is the shown file's rendered state, present when IsFile is true.
	File StoreFile
	// Nav is the shared top-level navbar (navView) — the store page lights the
	// Store section.
	Nav NavView
}

// StoreCrumb is one breadcrumb segment: its label and the reverse-routed path to
// the directory it names (the root crumb points at store.index).
type StoreCrumb struct {
	Name string
	Path string
}

// StoreEntry is one child in a directory listing — its name, whether it is a
// directory, its size in bytes (files only), and the reverse-routed link to its
// own store.show page.
type StoreEntry struct {
	Name  string
	IsDir bool
	Size  int64
	Path  string
}

// StoreFile is a single file's rendered state (decision-012). Exactly one of
// IsText / TooLarge / IsBinary is true. Text carries the escaped-by-htmlc
// contents when the file is UTF-8 text under the inline size cap; otherwise the
// view offers a download (RawPath, the same route with ?raw=1).
type StoreFile struct {
	Size     int64
	IsText   bool
	Text     string
	TooLarge bool
	IsBinary bool
	// RawPath is the reverse-routed download link — store.show with ?raw=1.
	RawPath string
}

// RenderStore writes the full store browse page (docs/08).
func RenderStore(ctx context.Context, w io.Writer, engine *htmlc.Engine, view StoreView) error {
	return engine.RenderPage(ctx, w, "view_store", map[string]any{
		"store": view,
	})
}
