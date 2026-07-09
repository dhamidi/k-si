package apps

// App — one registered app: name (slug/unit), port, start command, raw
// app.json operations, and status (registered|running|removing) (docs/15,
// feature-apps.md).
type App struct {
	// Name is the app's identity — a slug, also the systemd unit's name
	// (feature-apps.md).
	Name string `json:"name"`
	Port int    `json:"port"`
	// StartCmd is run with $PORT in its environment; käsi never parses it.
	StartCmd string `json:"start_cmd"`
	// Operations is the RAW app.json contents, verbatim, unparsed — the agent
	// edge re-parses it when it aggregates apps.json (store raw, derive on
	// replay, like a memory's Content vs its remember directive).
	Operations string `json:"operations"`
	URL        string `json:"url"`
	Status     string `json:"status"`
}

const (
	// StatusRegistered — recorded in the log; the apps-reconcile subscription
	// hasn't yet made the systemd unit match (decision-015).
	StatusRegistered = "registered"
	// StatusRunning — the systemd unit is up and matches the registration.
	StatusRunning = "running"
	// StatusRemoving — rm requested; the unit is being torn down.
	StatusRemoving = "removing"
)
