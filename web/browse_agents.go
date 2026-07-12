package web

import (
	"log"
	"net/http"
	"strings"

	"github.com/dhamidi/k-si/agents"
	agentmsg "github.com/dhamidi/k-si/agents/msg"
)

// showAgents renders /agents (decision-024): the section that lists every worker
// agent with its connection status and lets the operator pick the default one new
// tasks run on. Host-gated, no token (decision-006). Claude is built in; Codex is
// "signed in" exactly when its reserved credential is present (managed at /codex).
func (s *Server) showAgents(w http.ResponseWriter, r *http.Request) {
	v := s.app.View()
	def := agents.WorkerHarnessOf(v)
	if def == "" {
		def = agents.DefaultHarness
	}
	connected := s.codexConnected()
	codexPath, _ := s.router.Path("codex.index", nil)

	var rows []AgentRow
	var opts []AgentOption
	for _, name := range agents.HarnessNames() {
		label := agentLabel(name)
		opts = append(opts, AgentOption{Value: name, Label: label, Selected: name == def})
		row := AgentRow{Label: label, Default: name == def}
		switch name {
		case "codex":
			if connected {
				row.Status = "Signed in with ChatGPT"
				row.ManageLabel = "Manage sign-in"
			} else {
				row.Status = "Not signed in"
				row.ManageLabel = "Sign in to Codex"
			}
			row.ManagePath = codexPath
		default: // Claude and any other built-in agent: runs on the machine's login
			row.Status = "Built in — ready"
		}
		rows = append(rows, row)
	}

	warning := ""
	if def == "codex" && !connected {
		warning = "Codex is the default agent but is not signed in — sign in below, or new tasks will fail to run."
	}

	save, _ := s.router.Path("agents.save", nil)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := RenderAgents(r.Context(), w, s.engine, AgentsView{
		Agents:   rows,
		Options:  opts,
		Warning:  warning,
		SavePath: save,
		Nav:      s.navView("agents.index"),
	}); err != nil {
		log.Printf("web: render agents: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// saveDefaultAgent records the operator's chosen default agent as one imperative
// message (the same set-worker-harness the setting emits), then redirects back so
// the GET shows the new default (App.Send blocks until applied). An unknown name
// is rejected — the select only offers real harnesses.
func (s *Server) saveDefaultAgent(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("agent")
	known := false
	for _, n := range agents.HarnessNames() {
		if n == name {
			known = true
			break
		}
	}
	if !known {
		http.Error(w, "unknown agent", http.StatusUnprocessableEntity)
		return
	}
	s.app.Send(agentmsg.NewSetWorkerHarness(agentmsg.SetWorkerHarnessPayload{Name: name}))
	path, _ := s.router.Path("agents.index", nil)
	http.Redirect(w, r, path, http.StatusSeeOther)
}

// agentLabel title-cases a harness name for display ("codex" → "Codex").
func agentLabel(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
