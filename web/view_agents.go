package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"
)

// AgentsView is the data view_agents.vue renders — the /agents section, the home
// for choosing which worker agent käsi runs and for connecting the ones that need
// it. Claude is built in (it runs on the machine's own login); Codex signs in with
// a ChatGPT subscription through /codex. This view carries only display strings and
// reverse-routed paths — no credential, no model object (decision-006/008).
type AgentsView struct {
	Agents  []AgentRow
	Options []AgentOption // the Default-agent <select>, one per selectable harness
	// Warning is shown when the default agent cannot currently run — e.g. Codex is
	// the default but not signed in. Empty when everything is in order.
	Warning  string
	SavePath string
	Nav      NavView
}

// AgentRow is one agent's row: its name, whether it is the current default, its
// connection status, and (for an agent that needs connecting) where to manage it.
type AgentRow struct {
	Label       string
	Status      string
	Default     bool
	ManagePath  string // "" for an agent with nothing to manage (Claude)
	ManageLabel string
}

// AgentOption is one choice in the Default-agent select.
type AgentOption struct {
	Value    string
	Label    string
	Selected bool
}

// RenderAgents renders the /agents page.
func RenderAgents(ctx context.Context, w io.Writer, engine *htmlc.Engine, view AgentsView) error {
	return engine.RenderPage(ctx, w, "view_agents", map[string]any{"agents": view})
}
