// Package web is the web edge (docs/08): a server-rendered hypermedia UI —
// dispatch routes htmlc-rendered pages; writes are form objects that become
// runtime messages. This is the barebones cut that proves the stack over the
// counter canary; the real pages grow here in stage 3 (BUILDING.md).
package web

import (
	"crypto/subtle"
	"embed"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/dhamidi/dispatch"
	"github.com/dhamidi/htmlc"

	"github.com/dhamidi/k-si/counter"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/tasks"
	taskmsg "github.com/dhamidi/k-si/tasks/msg"
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
	// The completion link — the one routine interaction that leaves email for the
	// web (docs/04). A capability URL: the unguessable token IS the authorisation.
	if err := s.router.GET("tasks.done", "/tasks/{id}/done", http.HandlerFunc(s.finishTask)); err != nil {
		return nil, err
	}

	return s, nil
}

// finishTask handles the completion link in an email reply (docs/04, docs/08):
// it validates the capability token against the task's, emits finish-task
// through the one front door, and confirms. The token check is constant-time —
// the token is the only credential.
func (s *Server) finishTask(w http.ResponseWriter, r *http.Request) {
	params, _ := dispatch.ParamsFromContext(r.Context())
	id, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	token := r.URL.Query().Get("token")

	task, ok := tasks.Get(s.app.View(), tasks.TaskID(id))
	if !ok || task.CompletionToken == "" ||
		subtle.ConstantTimeCompare([]byte(task.CompletionToken), []byte(token)) != 1 {
		http.Error(w, "invalid or expired link", http.StatusNotFound)
		return
	}

	// Idempotent: clicking an already-done link just re-confirms (App.Send blocks
	// until applied, so a re-render would see the final state anyway).
	if task.Status != tasks.Done {
		s.app.Send(taskmsg.NewFinishTask(taskmsg.FinishTaskPayload{TaskID: id}))
	}

	// Minimal confirmation — the real task views land in stage 3 (BUILDING).
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8">`+
		`<meta name="viewport" content="width=device-width,initial-scale=1"><title>Task done</title></head>`+
		`<body style="font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem">`+
		`<h1>Done ✓</h1><p>This task has been marked complete. You can close this page.</p></body></html>`)
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
