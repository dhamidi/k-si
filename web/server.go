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
	"net/url"
	"strconv"

	"github.com/dhamidi/dispatch"
	"github.com/dhamidi/htmlc"

	"github.com/dhamidi/k-si/counter"
	"github.com/dhamidi/k-si/link"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/secrets"
	"github.com/dhamidi/k-si/store"
	"github.com/dhamidi/k-si/tasks"
	taskmsg "github.com/dhamidi/k-si/tasks/msg"
	"github.com/dhamidi/k-si/workspace"
)

//go:embed *.vue
var templates embed.FS

// SecretWriter writes a secret plaintext to the secrets store under a
// secret:// URL, returning nothing but the write's success (decision-004). The
// web edge holds only this narrow capability — it writes at answer time; the
// plaintext never enters the model, a message, the view, or a log line. The
// production SQLiteSecrets and the sim's SimSecrets both satisfy it.
type SecretWriter interface {
	Set(url, plaintext string) error
}

// Server binds the router, the template engine, and the running App.
// Reads render View structs from the in-RAM model; writes go through form
// objects into App.Send, which blocks until applied — so the redirected GET
// always shows the new state (docs/08).
type Server struct {
	app     *runtime.App
	engine  *htmlc.Engine
	router  *dispatch.Router
	secrets SecretWriter
	content store.Content
	// work is the filesystem edge for reading a running run's in-progress
	// transcript; a finished run reads from content instead (decision-007).
	work workspace.Workspace
}

// NewServer wires the running App plus the edge-I/O capabilities the pages need:
// secrets writes a UI-request secret field's plaintext and hands back a reference;
// content stores uploaded files and reads archived transcripts/artifacts; work
// reads a running run's in-progress transcript from its workspace (decision-007).
// The supervisor owns the one call site.
func NewServer(app *runtime.App, secrets SecretWriter, content store.Content, work workspace.Workspace) (*Server, error) {
	engine, err := htmlc.New(htmlc.Options{FS: templates, ComponentDir: "."})
	if err != nil {
		return nil, err
	}

	s := &Server{app: app, engine: engine, router: dispatch.New(), secrets: secrets, content: content, work: work}

	if err := s.router.GET("counter.show", "/", http.HandlerFunc(s.showCounter)); err != nil {
		return nil, err
	}
	if err := s.router.POST("counter.increment", "/increment", http.HandlerFunc(s.increment)); err != nil {
		return nil, err
	}
	// The completion link — the one routine interaction that leaves email for the
	// web (docs/04). A capability URL: the unguessable token IS the authorisation.
	if err := s.router.GET(link.CompletionRoute, link.CompletionPattern, http.HandlerFunc(s.finishTask)); err != nil {
		return nil, err
	}
	// The request link (Flow C): GET renders the spec-driven form (or the answered
	// state); POST, same capability URL, submits the answer (decision-003/005).
	if err := s.router.GET(link.RequestRoute, link.RequestPattern, http.HandlerFunc(s.showRequest)); err != nil {
		return nil, err
	}
	if err := s.router.POST("requests.answer", link.RequestPattern, http.HandlerFunc(s.answerRequest)); err != nil {
		return nil, err
	}
	// The browse UI (docs/08): the operator's window into the system. These
	// routes carry NO token — they are host-gated (decision-006), not
	// capability-linked. tasks.index lists, tasks.show details one task,
	// runs.transcript renders a run's session, runs.stop halts a running agent.
	if err := s.router.GET("tasks.index", "/tasks", http.HandlerFunc(s.showTasks)); err != nil {
		return nil, err
	}
	if err := s.router.GET("tasks.show", "/tasks/{id}", http.HandlerFunc(s.showTask)); err != nil {
		return nil, err
	}
	if err := s.router.GET("runs.transcript", "/tasks/{id}/runs/{run}/transcript", http.HandlerFunc(s.showTranscript)); err != nil {
		return nil, err
	}
	if err := s.router.POST("runs.stop", "/tasks/{id}/runs/{run}/stop", http.HandlerFunc(s.stopRun)); err != nil {
		return nil, err
	}
	// The skills browse UI (docs/08, Flow D decision-009/010): the registry list,
	// one skill's detail (metadata + file tree + SKILL.md), and any one tree entry's
	// raw text. Host-gated, no token (decision-006). skills.file's {+path} is a
	// reserved-expansion catch-all so a multi-segment relative path
	// (scripts/extract.sh) rides one segment, reverse-routed with clean slashes.
	if err := s.router.GET("skills.index", "/skills", http.HandlerFunc(s.showSkills)); err != nil {
		return nil, err
	}
	if err := s.router.GET("skills.show", "/skills/{name}", http.HandlerFunc(s.showSkill)); err != nil {
		return nil, err
	}
	if err := s.router.GET("skills.file", "/skills/{name}/files/{+path}", http.HandlerFunc(s.showSkillFile)); err != nil {
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
	token := r.URL.Query().Get(link.TokenParam)

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
	if err := RenderCompletion(r.Context(), w, s.engine, CompletionView{}); err != nil {
		log.Printf("web: render completion: %v", err)
	}
}

// requestCredential pulls the run id and capability token off the request-link
// route and validates the token, constant-time, against the record — the same
// trust model as the completion link (docs/04). A missing record, absent token,
// or mismatch is a 404, never a signal that the id exists. On success it returns
// the request record.
func (s *Server) requestCredential(w http.ResponseWriter, r *http.Request) (tasks.UIRequest, bool) {
	params, _ := dispatch.ParamsFromContext(r.Context())
	runID, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return tasks.UIRequest{}, false
	}
	token := r.URL.Query().Get(link.TokenParam)

	req, ok := tasks.RequestByRunID(s.app.View(), runID)
	if !ok || req.Token == "" ||
		subtle.ConstantTimeCompare([]byte(req.Token), []byte(token)) != 1 {
		http.Error(w, "invalid or expired link", http.StatusNotFound)
		return tasks.UIRequest{}, false
	}
	return req, true
}

// showRequest renders the spec-driven form the request link opens (Flow C,
// decision-005): the answered state once closed, otherwise the empty form built
// from the agent's spec.
func (s *Server) showRequest(w http.ResponseWriter, r *http.Request) {
	req, ok := s.requestCredential(w, r)
	if !ok {
		return
	}

	if req.Status == tasks.RequestAnswered {
		s.renderRequest(w, r, http.StatusOK, req, nil, FormErrors{})
		return
	}

	spec, err := ParseFormSpec(req.FormSpec)
	if err != nil {
		log.Printf("web: request %d: bad form spec: %v", req.RunID, err)
		http.Error(w, "this request cannot be displayed", http.StatusInternalServerError)
		return
	}
	s.renderRequest(w, r, http.StatusOK, req, spec, FormErrors{})
}

// answerRequest is the write loop for a UI-request answer (docs/08, decision-004):
// validate the token, bind + validate the spec-driven form; invalid re-renders
// (422) with the user's values and errors. Valid does the web-edge I/O — files to
// the archive, secrets to the secrets store, both by reference — then emits one
// answer-ui-request message and redirects back to the now-answered GET. No secret
// plaintext ever reaches the view, the message, the redirect, or a log line.
func (s *Server) answerRequest(w http.ResponseWriter, r *http.Request) {
	req, ok := s.requestCredential(w, r)
	if !ok {
		return
	}

	// Already answered: the link is spent; re-tapping shows the closed state.
	if req.Status == tasks.RequestAnswered {
		http.Redirect(w, r, s.requestAction(req.RunID, req.Token), http.StatusSeeOther)
		return
	}

	spec, err := ParseFormSpec(req.FormSpec)
	if err != nil {
		log.Printf("web: request %d: bad form spec: %v", req.RunID, err)
		http.Error(w, "this request cannot be displayed", http.StatusInternalServerError)
		return
	}

	form := BindAnswerRequestForm(r, spec).Validate()
	if !form.Valid() {
		s.renderRequestForm(w, r, http.StatusUnprocessableEntity, req, spec, form)
		return
	}

	// Web-edge I/O (decision-004): references into the model, heavy content and
	// plaintext out to the archive and the secrets store.
	values := map[string]string{}
	fileRefs := map[string]int64{}
	secretRefs := map[string]string{}

	for name, v := range form.Values {
		values[name] = v
	}
	for name, f := range form.Files {
		id, err := s.content.AddArchive(store.ArchiveRow{
			TaskID:      req.TaskID,
			Kind:        "attachment",
			Filename:    f.Filename,
			ContentType: f.ContentType,
			Bytes:       f.Bytes,
		})
		if err != nil {
			log.Printf("web: request %d: archive %q: %v", req.RunID, name, err)
			http.Error(w, "could not store the uploaded file", http.StatusInternalServerError)
			return
		}
		fileRefs[name] = id
	}
	for name, plaintext := range form.Secrets {
		u := secrets.URL(fmt.Sprintf("task/%d", req.TaskID), name)
		if err := s.secrets.Set(u, plaintext); err != nil {
			// Do not log the field value; only that the write failed.
			log.Printf("web: request %d: secret write failed", req.RunID)
			http.Error(w, "could not store a submitted secret", http.StatusInternalServerError)
			return
		}
		secretRefs[name] = u
	}

	// App.Send blocks until applied, so the redirected GET sees the answered
	// state (docs/08).
	s.app.Send(taskmsg.NewAnswerUIRequest(taskmsg.AnswerUIRequestPayload{
		TaskID:     req.TaskID,
		RunID:      req.RunID,
		Values:     values,
		FileRefs:   fileRefs,
		SecretRefs: secretRefs,
	}))

	http.Redirect(w, r, s.requestAction(req.RunID, req.Token), http.StatusSeeOther)
}

// requestAction builds the capability URL for a request — the form's POST target
// and the GET the answered redirect returns to. The token rides the query, as on
// the completion link (decision-003); it is never a secret, so this is safe.
func (s *Server) requestAction(runID int64, token string) string {
	path, _ := s.router.Path(link.RequestRoute, dispatch.Params{"id": strconv.FormatInt(runID, 10)})
	u := url.URL{Path: path}
	q := u.Query()
	q.Set(link.TokenParam, token)
	u.RawQuery = q.Encode()
	return u.String()
}

// renderRequest renders the page from a request record: fields empty (a fresh
// GET) or omitted (the answered state). renderRequestForm is the invalid-submit
// variant that echoes the form's values and errors.
func (s *Server) renderRequest(w http.ResponseWriter, r *http.Request, status int, req tasks.UIRequest, spec []FieldSpec, errs FormErrors) {
	view := RequestView{
		Message:  requestMessage(req),
		Fields:   fieldViews(spec, nil, nil, errs),
		Action:   s.requestAction(req.RunID, req.Token),
		Answered: req.Status == tasks.RequestAnswered,
	}
	s.writeRequest(w, r, status, view)
}

func (s *Server) renderRequestForm(w http.ResponseWriter, r *http.Request, status int, req tasks.UIRequest, spec []FieldSpec, form AnswerRequestForm) {
	view := RequestView{
		Message:  requestMessage(req),
		Fields:   fieldViews(spec, form.Values, form.Files, form.Errors),
		Action:   s.requestAction(req.RunID, req.Token),
		Answered: false,
	}
	s.writeRequest(w, r, status, view)
}

func (s *Server) writeRequest(w http.ResponseWriter, r *http.Request, status int, view RequestView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := RenderRequest(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render request: %v", err)
	}
}

// fieldViews turns the spec into per-field render data (decision-005). values
// echoes text/longtext/choice back on an invalid re-render; a file field echoes
// only its remembered filename (a browser cannot be re-seeded with the bytes) and
// a secret NEVER echoes a value (decision-004). errs supplies per-field messages.
func fieldViews(spec []FieldSpec, values map[string]string, files map[string]UploadedFile, errs FormErrors) []FieldView {
	views := make([]FieldView, 0, len(spec))
	for _, f := range spec {
		fv := FieldView{
			Name:     f.Name,
			Label:    f.Label,
			Type:     f.Type,
			Required: f.Required,
			Options:  f.Options,
			Error:    errs[f.Name],
		}
		switch f.Type {
		case FieldSecret:
			// never echo a plaintext back into the page
		case FieldFile:
			if up, ok := files[f.Name]; ok {
				fv.Value = up.Filename
			}
		default:
			fv.Value = values[f.Name]
		}
		views = append(views, fv)
	}
	return views
}

// requestMessage is the summary the page leads with. Stage-3 requests carry no
// separate prose field on the record, so this states the ask plainly; a richer
// message can flow through the spec later without changing the view.
func requestMessage(req tasks.UIRequest) string {
	return "An agent working on your task needs the following input to continue."
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
