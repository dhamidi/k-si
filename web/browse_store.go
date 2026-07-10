package web

import (
	"bytes"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dhamidi/dispatch"
)

// storeMaxInlineBytes caps how much of a text file the store page inlines. A
// larger file shows a "too large" notice and a download link instead of pushing
// a megabyte of bytes into the rendered page.
const storeMaxInlineBytes = 512 * 1024

// showStore renders the store root listing (docs/08, Flow F decision-012).
// Host-gated, no token (decision-006), READ-ONLY.
func (s *Server) showStore(w http.ResponseWriter, r *http.Request) {
	s.renderStoreDir(w, r, ".")
}

// showStorePath renders one subpath of the store: a directory lists, a file
// shows (decision-012). The {+path} catch-all carries a multi-segment store path
// (accounting/ledger.db) percent-encoded, so it is unescaped, then validated
// with fs.ValidPath — an invalid path (absolute, "..", empty) is a 404, and the
// fs.FS is already rooted so traversal out of the store is impossible. A missing
// path or read error degrades to a friendly 404, never a 500.
func (s *Server) showStorePath(w http.ResponseWriter, r *http.Request) {
	params, _ := dispatch.ParamsFromContext(r.Context())
	p, err := url.PathUnescape(params["path"])
	if err != nil {
		p = params["path"]
	}
	if p == "." || !fs.ValidPath(p) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if s.store == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// A directory reads cleanly; a file fails the directory read (not a
	// directory), so fall through to the file view. This is what lets the sim
	// twin — which has no Stat for directories — still distinguish the two.
	if entries, err := fs.ReadDir(s.store, p); err == nil {
		s.writeStoreDir(w, r, p, entries)
		return
	}
	s.renderStoreFile(w, r, p)
}

// renderStoreDir builds and writes a directory listing. dir is "." for the root.
// A nil store, or a root that cannot be read, renders an empty page rather than
// failing; a named subdirectory that cannot be read is a 404.
func (s *Server) renderStoreDir(w http.ResponseWriter, r *http.Request, dir string) {
	var entries []fs.DirEntry
	if s.store != nil {
		e, err := fs.ReadDir(s.store, dir)
		if err != nil {
			if dir != "." {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			// The root of an empty or absent store: show the empty state.
			e = nil
		}
		entries = e
	}
	s.writeStoreDir(w, r, dir, entries)
}

// writeStoreDir renders a directory's children into the page: dirs first, then
// files, each sorted, each a reverse-routed link to its own store.show page
// (rule no-url-string-building).
func (s *Server) writeStoreDir(w http.ResponseWriter, r *http.Request, dir string, entries []fs.DirEntry) {
	rows := make([]StoreEntry, 0, len(entries))
	for _, e := range entries {
		full := e.Name()
		if dir != "." {
			full = dir + "/" + e.Name()
		}
		var size int64
		if info, err := e.Info(); err == nil {
			size = info.Size()
		}
		rows = append(rows, StoreEntry{
			Name:  e.Name(),
			IsDir: e.IsDir(),
			Size:  size,
			Path:  s.storeShowPath(full),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].IsDir != rows[j].IsDir {
			return rows[i].IsDir // directories first
		}
		return rows[i].Name < rows[j].Name
	})

	view := StoreView{
		Path:    storeDisplayPath(dir),
		Crumbs:  s.storeCrumbs(dir),
		IsFile:  false,
		Entries: rows,
		Empty:   len(rows) == 0,
		Nav:     s.navView("store.index"),
	}
	s.writeStore(w, r, view)
}

// renderStoreFile shows one file (decision-012): its text inline when it is
// UTF-8 text under the size cap, else a "too large" notice or a "binary file"
// download link. ?raw=1 streams the raw bytes as an attachment instead. A read
// error is a 404, never a 500 stack.
func (s *Server) renderStoreFile(w http.ResponseWriter, r *http.Request, p string) {
	info, err := fs.Stat(s.store, p)
	if err != nil || info.IsDir() {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if r.URL.Query().Get("raw") == "1" {
		s.streamStoreRaw(w, p)
		return
	}

	size := info.Size()
	file := StoreFile{Size: size, RawPath: s.storeRawPath(p)}
	if size > storeMaxInlineBytes {
		file.TooLarge = true
	} else {
		b, err := fs.ReadFile(s.store, p)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if isStoreText(b) {
			file.IsText = true
			file.Text = string(b)
		} else {
			file.IsBinary = true
		}
	}

	view := StoreView{
		Path:   p,
		Crumbs: s.storeCrumbs(p),
		IsFile: true,
		File:   file,
		Nav:    s.navView("store.index"),
	}
	s.writeStore(w, r, view)
}

// streamStoreRaw streams a store file's bytes as a download (decision-012): an
// application/octet-stream attachment, so a binary file leaves the browser
// rather than mangling the page. Read-only, host-gated like the rest.
func (s *Server) streamStoreRaw(w http.ResponseWriter, p string) {
	f, err := s.store.Open(p)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+path.Base(p)+"\"")
	if _, err := io.Copy(w, f); err != nil {
		log.Printf("web: store raw %q: %v", p, err)
	}
}

func (s *Server) writeStore(w http.ResponseWriter, r *http.Request, view StoreView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := RenderStore(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render store: %v", err)
	}
}

// storeCrumbs builds the breadcrumb trail from the store root down to p: the
// root crumb links to store.index, each subsequent crumb to store.show for its
// accumulated path (rule no-url-string-building).
func (s *Server) storeCrumbs(p string) []StoreCrumb {
	crumbs := []StoreCrumb{{Name: "store", Path: s.storeIndexPath()}}
	if p == "." || p == "" {
		return crumbs
	}
	acc := ""
	for _, seg := range strings.Split(p, "/") {
		if acc == "" {
			acc = seg
		} else {
			acc = acc + "/" + seg
		}
		crumbs = append(crumbs, StoreCrumb{Name: seg, Path: s.storeShowPath(acc)})
	}
	return crumbs
}

// storeIndexPath reverse-routes the store root.
func (s *Server) storeIndexPath() string {
	p, _ := s.router.Path("store.index", nil)
	return p
}

// storeShowPath reverse-routes a store subpath — the {+path} catch-all preserves
// the multi-segment path with clean slashes (accounting/ledger.db).
func (s *Server) storeShowPath(p string) string {
	path, _ := s.router.Path("store.show", dispatch.Params{"path": p})
	return path
}

// storeRawPath reverse-routes a store file's download link — store.show with
// ?raw=1, built through net/url, never string-concatenated (rule
// no-url-string-building).
func (s *Server) storeRawPath(p string) string {
	base, _ := s.router.Path("store.show", dispatch.Params{"path": p})
	u := url.URL{Path: base}
	q := u.Query()
	q.Set("raw", "1")
	u.RawQuery = q.Encode()
	return u.String()
}

// storeDisplayPath renders the current path for the heading — the root (".") has
// no path to show.
func storeDisplayPath(dir string) string {
	if dir == "." {
		return ""
	}
	return dir
}

// isStoreText decides whether a file inlines as text: no NUL byte and valid
// UTF-8. Everything else is treated as binary and offered as a download.
func isStoreText(b []byte) bool {
	if bytes.IndexByte(b, 0) >= 0 {
		return false
	}
	return utf8.Valid(b)
}
