package web

import (
	"log"
	"net/http"

	"github.com/dhamidi/dispatch"

	"github.com/dhamidi/k-si/memory"
	memorymsg "github.com/dhamidi/k-si/memory/msg"
)

// showMemory renders the memory curation page (docs/08, feature-memory.md): every
// remembered fact with its derived description and raw content, an add/edit form,
// and a per-row forget action. Host-gated, no token (decision-006). The list reads
// the model (memory.All); the descriptions were derived by the reducer on replay.
func (s *Server) showMemory(w http.ResponseWriter, r *http.Request) {
	s.renderMemory(w, r, http.StatusOK, RememberForm{Errors: FormErrors{}})
}

// renderMemory writes the page from the current collection plus a form object —
// empty on a plain GET, echoed with values and errors on an invalid submit.
func (s *Server) renderMemory(w http.ResponseWriter, r *http.Request, status int, form RememberForm) {
	all := memory.All(s.app.View())

	tasksPath, _ := s.router.Path("tasks.index", nil)
	view := MemoryView{
		Memories:  memoryRows(all, s.memoryForgetPath),
		Form:      form,
		SavePath:  s.memorySavePath(),
		TasksPath: tasksPath,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := RenderMemory(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render memory: %v", err)
	}
}

// saveMemory is the write loop for the owner's add/edit (docs/08): bind + validate
// the remember form; invalid re-renders (422) with the values and errors; valid
// becomes exactly one remember message, then POST/redirect/GET. App.Send blocks
// until applied, so the redirected GET shows the new collection.
func (s *Server) saveMemory(w http.ResponseWriter, r *http.Request) {
	form := BindRememberForm(r).Validate()
	if !form.Valid() {
		s.renderMemory(w, r, http.StatusUnprocessableEntity, form)
		return
	}

	s.app.Send(form.Message())

	index, _ := s.router.Path("memory.index", nil)
	http.Redirect(w, r, index, http.StatusSeeOther)
}

// forgetMemory removes a memory straight from the list — the owner's curation
// counterpart of an agent's in/ deletion (decision-006, no token). It emits forget
// and redirects back; App.Send blocks until applied, so the redirected GET shows
// the memory gone. Idempotent: forgetting an absent name is a no-op in the reducer.
func (s *Server) forgetMemory(w http.ResponseWriter, r *http.Request) {
	params, _ := dispatch.ParamsFromContext(r.Context())
	name := params["name"]
	if name == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	s.app.Send(memorymsg.NewForget(memorymsg.ForgetPayload{Name: name}))

	index, _ := s.router.Path("memory.index", nil)
	http.Redirect(w, r, index, http.StatusSeeOther)
}

// memorySavePath reverse-routes the remember form's POST target (rule
// no-url-string-building).
func (s *Server) memorySavePath() string {
	p, _ := s.router.Path("memory.save", nil)
	return p
}

// memoryForgetPath reverse-routes a memory's forget POST target for its name.
func (s *Server) memoryForgetPath(name string) string {
	p, _ := s.router.Path("memory.forget", dispatch.Params{"name": name})
	return p
}
