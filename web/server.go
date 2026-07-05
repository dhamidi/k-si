// Package web is the web edge (docs/08): a server-rendered hypermedia UI —
// dispatch routes htmlc-rendered pages; writes are form objects that become
// runtime messages. This is the barebones cut that proves the stack over the
// counter canary; the real pages grow here in stage 3 (BUILDING.md).
package web

import (
	"embed"
	"log"
	"net/http"

	"github.com/dhamidi/dispatch"
	"github.com/dhamidi/htmlc"

	"github.com/dhamidi/k-si/counter"
	"github.com/dhamidi/k-si/runtime"
)

//go:embed view_*.vue
var templates embed.FS

// Server binds the router, the template engine, and the running App.
// Reads render View structs from the in-RAM model; writes go through form
// objects into App.Send, which blocks until applied — so the redirected GET
// always shows the new state (docs/08).
type Server struct {
	app    *runtime.App
	engine *htmlc.Engine
	router *dispatch.Router
}

func NewServer(app *runtime.App) (*Server, error) {
	engine, err := htmlc.New(htmlc.Options{FS: templates, ComponentDir: "."})
	if err != nil {
		return nil, err
	}

	s := &Server{app: app, engine: engine, router: dispatch.New()}

	if err := s.router.GET("counter.show", "/", http.HandlerFunc(s.showCounter)); err != nil {
		return nil, err
	}
	if err := s.router.POST("counter.increment", "/increment", http.HandlerFunc(s.increment)); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) showCounter(w http.ResponseWriter, r *http.Request) {
	s.renderCounter(w, r, http.StatusOK, IncrementCounterForm{Errors: FormErrors{}})
}

// increment is the whole write loop of docs/08: bind + validate; invalid
// re-renders the same view with the form's values and errors; valid becomes
// exactly one message, then POST/redirect/GET.
func (s *Server) increment(w http.ResponseWriter, r *http.Request) {
	form := BindIncrementCounterForm(r).Validate()

	if !form.Valid() {
		s.renderCounter(w, r, http.StatusUnprocessableEntity, form)
		return
	}

	s.app.Send(form.Message())

	show, _ := s.router.Path("counter.show", nil)
	http.Redirect(w, r, show, http.StatusSeeOther)
}

func (s *Server) renderCounter(w http.ResponseWriter, r *http.Request, status int, form IncrementCounterForm) {
	increment, _ := s.router.Path("counter.increment", nil)

	view := CounterView{
		Count:         counter.Count(s.app.View()),
		Form:          form,
		IncrementPath: increment,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	if err := RenderCounter(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render counter: %v", err)
	}
}
