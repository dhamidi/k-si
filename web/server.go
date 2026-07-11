// Package web is the web edge (docs/08): a server-rendered hypermedia UI —
// dispatch routes htmlc-rendered pages; writes are form objects that become
// runtime messages. This is the barebones cut that proves the stack over the
// counter canary; the real pages grow here in stage 3 (BUILDING.md).
package web

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"

	"github.com/dhamidi/dispatch"
	"github.com/dhamidi/htmlc"

	"github.com/dhamidi/k-si/agents"
	"github.com/dhamidi/k-si/apps"
	appsmsg "github.com/dhamidi/k-si/apps/msg"
	"github.com/dhamidi/k-si/counter"
	"github.com/dhamidi/k-si/link"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/secrets"
	"github.com/dhamidi/k-si/settings"
	"github.com/dhamidi/k-si/store"
	"github.com/dhamidi/k-si/tasks"
	taskmsg "github.com/dhamidi/k-si/tasks/msg"
	"github.com/dhamidi/k-si/workspace"
)

//go:embed *.vue
var templates embed.FS

// turboJS is the vendored Turbo runtime (Turbo 8.x), served on the assets.turbo
// route as the ONE script include (docs/16, decision-020). Turbo is a progressive
// enhancement: present, it swaps a <turbo-frame> in place on a reshape; absent,
// the same POST reloads the page. No test exercises the JS — the reshape works
// without it via the full-page branch — so the file is drop-in and swappable.
//
//go:embed turbo.min.js
var turboJS []byte

// SecretStore is the web edge's narrow capability over the secrets store
// (decision-004): it writes a plaintext under a secret:// reference, deletes a
// reference, and lists the stored references with their last-set time — NEVER a
// value (secrets.Entry carries only a Ref and UpdatedAt). Set writes at the web
// edge and the plaintext is dropped; it never enters the model, a message, the
// view, a URL, or a log line. Entries and Delete deal in references alone. The
// production SQLiteSecrets and the sim's SimSecrets both satisfy it.
type SecretStore interface {
	Set(url, plaintext string) error
	Delete(url string) error
	Entries() ([]secrets.Entry, error)
}

// Server binds the router, the template engine, and the running App.
// Reads render View structs from the in-RAM model; writes go through form
// objects into App.Send, which blocks until applied — so the redirected GET
// always shows the new state (docs/08).
type Server struct {
	app     *runtime.App
	engine  *htmlc.Engine
	router  *dispatch.Router
	secrets SecretStore
	content store.Content
	// work is the filesystem edge for reading a running run's in-progress
	// transcript; a finished run reads from content instead (decision-007).
	work workspace.Workspace
	// runner reads an app's live state from the machine for the /apps page
	// (feature-apps.md); nil until the apprunner edge is wired (docs/15), in
	// which case the page still renders the registry, with liveness "unknown".
	runner AppRunner
	// store is käsi's READ-ONLY view of the agent's persistent store directory
	// (Flow F, decision-012) — what the /store browse page walks. A datastore.Store
	// satisfies it; production passes the real os.DirFS-backed store, scenarios the
	// in-memory sim twin. Host-gated, never written through (a browse page reads).
	// Nil renders an empty store.
	store fs.FS
	// appsOrigin is the public scheme+host (no port) apps are addressed under —
	// e.g. "https://vm.exe.xyz" — set by SetAppsOrigin from the server's base URL
	// (feature-apps.md). Empty falls back to http://localhost:<port>/, which is
	// the loopback address a locally-run app answers on.
	appsOrigin string
	// settings is the typed-setting contribution list the /settings surface renders
	// and writes (docs/16, decision-020). Assembled in the open by main.go (web.Settings)
	// and handed in here; there is no registry. Each is a read + a write over state
	// that stays in its owning module's slice.
	settings []settings.Setting
	// codexSignIn launches the host-gated Codex sign-in (decision-025); nil until
	// SetCodexSignIn wires a twin (the real device-auth launcher in production, the
	// sim in scenarios), in which case the connect action reports it is unavailable.
	// codexSession is the one sign-in the server may be holding — at most one runs at
	// a time (the operator signs in once), so it needs no id; codexMu guards it.
	codexSignIn  CodexSignIn
	codexMu      sync.Mutex
	codexSession CodexSignInSession
}

// Settings concatenates each module's settings contribution into the one slice the
// server renders and writes (docs/16, decision-020) — the assembly convenience
// main.go calls as web.Settings(admin.Settings(), tasks.Settings(), agents.Settings()).
// No registry, no init(): the list is built in the open, beside the module list it
// mirrors.
func Settings(groups ...[]settings.Setting) []settings.Setting {
	var all []settings.Setting
	for _, g := range groups {
		all = append(all, g...)
	}
	return all
}

// appNameRE is the slug an app name must be: a systemd unit name and a URL
// segment (feature-apps.md). Lowercase alphanumeric, then alphanumerics,
// hyphens, or underscores.
var appNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// SetAppsOrigin records the public scheme+host apps are reachable under, so the
// control endpoint mints a public-correct app URL (feature-apps.md). origin is a
// scheme+host with no port (e.g. https://vm.exe.xyz); the port is appended per
// app. The supervisor derives it from -base-url after NewServer, keeping that
// signature untouched.
func (s *Server) SetAppsOrigin(origin string) {
	s.appsOrigin = origin
}

// appURL builds an app's public URL at the given forwarded port: <origin>:<port>/
// when an apps origin is set, else the loopback http://localhost:<port>/
// (feature-apps.md). Built through net/url, never string-concatenated
// (rule no-url-string-building).
func (s *Server) appURL(port int) string {
	u := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort("localhost", strconv.Itoa(port)),
		Path:   "/",
	}
	if s.appsOrigin != "" {
		if origin, err := url.Parse(s.appsOrigin); err == nil && origin.Hostname() != "" {
			u.Scheme = origin.Scheme
			u.Host = net.JoinHostPort(origin.Hostname(), strconv.Itoa(port))
		}
	}
	return u.String()
}

// NewServer wires the running App plus the edge-I/O capabilities the pages need:
// secrets writes a UI-request secret field's plaintext and hands back a reference;
// content stores uploaded files and reads archived transcripts/artifacts; work
// reads a running run's in-progress transcript from its workspace (decision-007);
// runner reads an app's live status/logs for the /apps page (feature-apps.md).
// The supervisor owns the one call site.
func NewServer(app *runtime.App, secrets SecretStore, content store.Content, work workspace.Workspace, runner AppRunner, storeFS fs.FS, settingList []settings.Setting) (*Server, error) {
	engine, err := htmlc.New(htmlc.Options{FS: templates, ComponentDir: "."})
	if err != nil {
		return nil, err
	}
	// base_styles.vue includes the Turbo <script> only when a page passes it a
	// `turbo` prop (the settings surface does); every other page omits the prop.
	// htmlc has no scope inheritance and no prop defaults, so an omitted prop must
	// resolve to something falsy without breaking the 13 other pages — hence a
	// scoped missing-prop handler: "turbo" absent → "" (script suppressed by the
	// v-if), every other missing prop keeps htmlc's default visible placeholder so
	// a genuine data bug still surfaces (docs/16).
	engine.WithMissingPropHandler(func(name string) (any, error) {
		if name == "turbo" {
			return "", nil
		}
		return fmt.Sprintf("[missing: %s]", name), nil
	})

	// SlashRedirect canonicalises a trailing slash: GET /tasks/ 308-redirects to
	// /tasks rather than 404ing (dispatch defaults to SlashIgnore, which treats the
	// two as distinct and matches neither). 308 (not the library's default 301)
	// preserves method and body, so a POST that arrives with a stray trailing slash
	// re-issues as a POST, not a GET.
	s := &Server{
		app: app, engine: engine, secrets: secrets, content: content, work: work, runner: runner, store: storeFS, settings: settingList,
		router: dispatch.New(
			dispatch.WithDefaultSlashPolicy(dispatch.SlashRedirect),
			dispatch.WithDefaultRedirectCode(http.StatusPermanentRedirect),
		),
	}

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
	// Same URL, POST: the host-gated "Done" action on the task list (decision-006,
	// no token) — the browser counterpart of the emailed completion link.
	if err := s.router.POST("tasks.markdone", link.CompletionPattern, http.HandlerFunc(s.markDone)); err != nil {
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
	// The memory curation UI (docs/08, feature-memory.md): GET lists every
	// remembered fact with an add/edit form; POST /memory is the owner's remember
	// (add or edit); POST /memory/{name}/forget removes one. Host-gated, no token
	// (decision-006) — the same trust model as the task/skill browse pages.
	if err := s.router.GET("memory.index", "/memory", http.HandlerFunc(s.showMemory)); err != nil {
		return nil, err
	}
	if err := s.router.POST("memory.save", "/memory", http.HandlerFunc(s.saveMemory)); err != nil {
		return nil, err
	}
	if err := s.router.POST("memory.forget", "/memory/{name}/forget", http.HandlerFunc(s.forgetMemory)); err != nil {
		return nil, err
	}
	// The apps browse page (docs/08, feature-apps.md): every registered app,
	// its URL, registry status, and live state. Host-gated, no token
	// (decision-006), read-only — registering/removing an app is `kasi app`'s
	// job, not this page's.
	if err := s.router.GET("apps.index", "/apps", http.HandlerFunc(s.showApps)); err != nil {
		return nil, err
	}
	// The store browse page (docs/08, Flow F decision-012): the operator's window
	// into the agent's persistent store directory. store.index lists the root;
	// store.show lists a subdirectory or shows a file — its {+path} is a
	// reserved-expansion catch-all, so a multi-segment store path
	// (accounting/ledger.db) rides one segment, reverse-routed with clean slashes,
	// and is validated with fs.ValidPath (an invalid path is a 404). A file's
	// download rides the same route with ?raw=1. Host-gated, no token
	// (decision-006), READ-ONLY — a browse page never writes.
	if err := s.router.GET("store.index", "/store", http.HandlerFunc(s.showStore)); err != nil {
		return nil, err
	}
	if err := s.router.GET("store.show", "/store/{+path}", http.HandlerFunc(s.showStorePath)); err != nil {
		return nil, err
	}
	// The secrets management surface (docs/06, decision-004): the index lists every
	// stored reference (name only, NEVER a value) with its last-set time, carries an
	// add/rotate form and a per-row delete, and shows the name-only audit trail. save
	// writes the plaintext at the edge and records the audit event; confirm-delete is
	// a no-JS safety step ("Delete <ref>? [Confirm] [Cancel]") so a critical
	// credential is not fat-fingered away; delete removes the reference and records
	// the removal. Host-gated, no token (decision-006). The plaintext touches ONLY the
	// edge Set call — never the view, a message, the log, or a URL (the reference,
	// which is safe, rides the delete round-trip in a hidden form field / query).
	if err := s.router.GET("secrets.index", "/secrets", http.HandlerFunc(s.showSecrets)); err != nil {
		return nil, err
	}
	if err := s.router.POST("secrets.save", "/secrets", http.HandlerFunc(s.saveSecret)); err != nil {
		return nil, err
	}
	if err := s.router.GET("secrets.confirm-delete", "/secrets/confirm-delete", http.HandlerFunc(s.confirmDeleteSecret)); err != nil {
		return nil, err
	}
	if err := s.router.POST("secrets.delete", "/secrets/delete", http.HandlerFunc(s.deleteSecret)); err != nil {
		return nil, err
	}
	// The settings surface (docs/16, decision-020): the index lists every
	// contributed setting; show renders one setting's form; reshape folds one
	// add/remove Event through a dynamic setting's form and re-renders (the frame,
	// or the whole page — content-negotiated on the Turbo-Frame header); save is the
	// final submit. All host-gated, no token (decision-006), reverse-routed.
	if err := s.router.GET("settings.index", "/settings", http.HandlerFunc(s.showSettings)); err != nil {
		return nil, err
	}
	if err := s.router.GET("settings.show", "/settings/{key}", http.HandlerFunc(s.showSetting)); err != nil {
		return nil, err
	}
	if err := s.router.POST("settings.reshape", "/settings/{key}/reshape", http.HandlerFunc(s.reshapeSetting)); err != nil {
		return nil, err
	}
	if err := s.router.POST("settings.save", "/settings/{key}", http.HandlerFunc(s.saveSetting)); err != nil {
		return nil, err
	}
	// The Codex sign-in surface (decision-025): käsi signs in to Codex once, on the
	// operator's behalf, and holds the result as a decision-004 secret the Codex
	// agent runs on. index shows the state (signed in / signing in / not signed in /
	// last sign-in expired); connect starts the host-gated device-auth and shows the
	// one-time public code + URL; poll re-checks a sign-in under way (the waiting
	// page's meta-refresh and "Check now" both land here) and harvests the credential
	// at the edge on success; disconnect signs out. Host-gated, no token
	// (decision-006); no inbound callback — codex polls out and the operator approves
	// in their own browser. The GET and POST on /codex/connect are distinct routes on
	// the one pattern (re-check vs start).
	if err := s.router.GET("codex.index", "/codex", http.HandlerFunc(s.showCodex)); err != nil {
		return nil, err
	}
	if err := s.router.POST("codex.connect", "/codex/connect", http.HandlerFunc(s.connectCodex)); err != nil {
		return nil, err
	}
	if err := s.router.GET("codex.poll", "/codex/connect", http.HandlerFunc(s.pollCodex)); err != nil {
		return nil, err
	}
	if err := s.router.POST("codex.disconnect", "/codex/disconnect", http.HandlerFunc(s.disconnectCodex)); err != nil {
		return nil, err
	}
	// The Turbo runtime, served from the embedded vendored file — the one script
	// the settings surface pulls (docs/16). A static asset: host-gated like the
	// rest, no token.
	if err := s.router.GET("assets.turbo", "/assets/turbo.min.js", http.HandlerFunc(s.serveTurbo)); err != nil {
		return nil, err
	}
	// The control endpoint (feature-notifications.md): `kasi notify` POSTs a mid-run
	// one-liner here. Host-gated (decision-006), and the per-run notify token is the
	// credential — the same trust model as the capability links, but for a one-way
	// mid-run signal rather than a round trip.
	if err := s.router.POST("control.notify", "/control/notify", http.HandlerFunc(s.notify)); err != nil {
		return nil, err
	}
	// The apps control endpoint (feature-apps.md): `kasi app add|rm` POSTs here to
	// register or remove an app mid-run. Host-gated (decision-006), and the per-run
	// token is the credential — the same trust model as `/control/notify`. The
	// endpoint records intent (register-app / unregister-app); the apps-reconcile
	// subscription makes the machine match (docs/15).
	if err := s.router.POST("control.app", "/control/app", http.HandlerFunc(s.controlApp)); err != nil {
		return nil, err
	}

	return s, nil
}

// app is the host-gated control endpoint behind `kasi app` (feature-apps.md): it
// validates the per-run token against the live AgentRun for the task, constant-
// time — exactly as notify does — then records the agent's intent. `add`
// registers (or re-registers) an app on a deterministically-assigned port and
// answers with its URL; `rm` unregisters it. A missing run and a wrong token both
// return 403, so the endpoint never leaks whether a task exists.
func (s *Server) controlApp(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	taskID, err := strconv.ParseInt(r.FormValue("task_id"), 10, 64)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	token := r.FormValue("token")
	action := r.FormValue("action")
	name := r.FormValue("name")
	// Every action but `ls` names a single app, which must be a slug (it becomes a
	// unit name and a URL segment). `ls` reads the whole registry and carries no
	// name, so it is exempt — the token is still required, same as the rest.
	if action != "ls" && !appNameRE.MatchString(name) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Find the live run for this task and validate the token constant-time. Not
	// found, empty token, or mismatch all return 403 — never a signal that the
	// task exists (mirrors s.notify).
	var run agents.AgentRun
	found := false
	for _, candidate := range agents.RunningRuns(s.app.View()) {
		if candidate.TaskID == taskID {
			run = candidate
			found = true
			break
		}
	}
	if !found || token == "" ||
		subtle.ConstantTimeCompare([]byte(run.NotifyToken), []byte(token)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	switch action {
	case "add":
		start := r.FormValue("start")
		operations := r.FormValue("operations")
		// Reuse the app's port on a re-add (a name is unique, feature-apps.md);
		// otherwise take the lowest free port. A full band is a 503.
		port := 0
		if existing, ok := apps.Find(s.app.View(), name); ok {
			port = existing.Port
		} else {
			port = apps.FreePort(s.app.View())
		}
		if port == 0 {
			http.Error(w, "no free port", http.StatusServiceUnavailable)
			return
		}
		u := s.appURL(port)
		s.app.Send(appsmsg.NewRegisterApp(appsmsg.RegisterAppPayload{
			Name:       name,
			Port:       port,
			StartCmd:   start,
			Operations: operations,
			URL:        u,
		}))
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, u)
	case "rm":
		s.app.Send(appsmsg.NewUnregisterApp(appsmsg.UnregisterAppPayload{Name: name}))
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	case "ls":
		// Read-only fleet listing: the registry from the log, as JSON. `kasi app
		// ls` renders this into a table. The token is still required (validated
		// above), the same trust model as add/rm.
		all := apps.All(s.app.View())
		rows := make([]appListRow, 0, len(all))
		for _, a := range all {
			rows = append(rows, appListRow{Name: a.Name, URL: a.URL, Status: a.Status, Port: a.Port})
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(rows); err != nil {
			log.Printf("web: control app ls: %v", err)
		}
	case "logs":
		// Live journald tail for one app, read through the narrow Runner edge. No
		// edge wired (the in-process test server), or an error reaching the
		// machine, both degrade to an empty body — logs are best-effort.
		w.WriteHeader(http.StatusOK)
		if s.runner == nil {
			return
		}
		n := 20
		if v := r.FormValue("n"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
				n = parsed
			}
		}
		lines, err := s.runner.Logs(r.Context(), name, n)
		if err != nil {
			log.Printf("web: control app logs %s: %v", name, err)
			return
		}
		for _, line := range lines {
			fmt.Fprintln(w, line)
		}
	case "restart":
		// Record the bounce as a directive (restart-app); the effect does the
		// systemctl restart. Going through the log keeps it auditable, exactly like
		// add/rm, rather than reaching the machine straight from the handler.
		s.app.Send(appsmsg.NewRestartApp(appsmsg.RestartAppPayload{Name: name}))
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	default:
		http.Error(w, "bad request", http.StatusBadRequest)
	}
}

// appListRow is the JSON shape `action=ls` returns per registered app — the
// slice `kasi app ls` renders into a table (feature-apps.md: the rest of the CLI
// manages the fleet).
type appListRow struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Status string `json:"status"`
	Port   int    `json:"port"`
}

// notify is the host-gated control endpoint behind `kasi notify` (feature-
// notifications.md): it validates the per-run token against the live AgentRun for
// the task, constant-time, then injects a notify-user message that emails the
// initiator a mid-run one-liner. The token is the only credential; a missing run
// and a wrong token both return 403, so the endpoint never leaks whether a task
// exists.
func (s *Server) notify(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	taskID, err := strconv.ParseInt(r.FormValue("task_id"), 10, 64)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	token := r.FormValue("token")
	message := r.FormValue("message")
	if message == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Find the live run for this task and validate the token constant-time. Not
	// found, empty token, or mismatch all return 403 — never a signal that the
	// task exists.
	var run agents.AgentRun
	found := false
	for _, candidate := range agents.RunningRuns(s.app.View()) {
		if candidate.TaskID == taskID {
			run = candidate
			found = true
			break
		}
	}
	if !found || token == "" ||
		subtle.ConstantTimeCompare([]byte(run.NotifyToken), []byte(token)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// App.Send blocks until applied; the mail fires fire-and-forget from the
	// reducer (feature-notifications.md).
	s.app.Send(taskmsg.NewNotifyUser(taskmsg.NotifyUserPayload{
		TaskID: taskID,
		RunID:  int64(run.ID),
		Body:   message,
	}))
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok")
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

// serveTurbo serves the vendored Turbo runtime as text/javascript (docs/16). A
// long-lived cache is safe: the file is content-stable and versioned by its
// vendored contents, and the enhancement degrades gracefully if it never loads.
func (s *Server) serveTurbo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if _, err := w.Write(turboJS); err != nil {
		log.Printf("web: serve turbo: %v", err)
	}
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
