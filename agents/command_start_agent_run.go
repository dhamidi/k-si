package agents

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strconv"

	"github.com/dhamidi/k-si/apps"
	"github.com/dhamidi/k-si/memory"
	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/skilltree"
	"github.com/dhamidi/k-si/workspace"
)

// localAppURL is the loopback origin an agent calls a registered app on — inside
// the auth boundary, no proxy (feature-apps.md). Built with net/url, not string
// concatenation (rule no-url-string-building).
func localAppURL(port int) string {
	u := url.URL{Scheme: "http", Host: net.JoinHostPort("localhost", strconv.Itoa(port))}
	return u.String()
}

// "start-agent-run" — start or resume the worker harness in the task workspace
const StartAgentRun = "start-agent-run"

type StartAgentRunPayload struct {
	TaskID int64 `json:"task_id"`
	RunID  int64 `json:"run_id"`
	Resume bool  `json:"resume"`
	// SecretRefs maps an env-var name to a secret:// URL, resolved into the run
	// environment at the harness edge (Flow C, docs/06). Carry-through only here.
	SecretRefs map[string]string `json:"secret_refs"`
	// Memory is the whole memory collection to provision into this run's in/ box
	// (feature-memory.md). Read from the model by the spawn handler and carried
	// through to the workspace edge here. This is a TRANSIENT effect input — a Cmd
	// payload, never appended to the log — so it may carry the raw note bytes
	// (provisioning the full collection each run must not grow the log).
	Memory []memory.Memory `json:"memory,omitempty"`
	// Apps is the set of running apps to advertise to this run as in/apps.json
	// (feature-apps.md, "the agent uses apps"). Like Memory, it is read from the
	// model by the launch handler and carried through to the workspace edge here —
	// a TRANSIENT effect input, never appended to the log, so it may carry the raw
	// app.json bytes.
	Apps []apps.App `json:"apps,omitempty"`
	// Harness is the run's pinned harness name (decision-024). The effect has no
	// View, so the launch handler resolves it from the run's pin and carries it
	// here; the effect dispatches through the registry by this name.
	Harness string `json:"harness,omitempty"`
	// Session is the run's resumable session (decision-024). On a resume the effect
	// hands it to Harness.Resume, so a harness that minted its own session id (Codex,
	// recorded via record-session) continues the RIGHT session rather than the
	// deterministic sessionFor.
	Session string `json:"session,omitempty"`
}

func NewStartAgentRun(p StartAgentRunPayload) runtime.Cmd {
	return runtime.NewCmd(StartAgentRun, p)
}

func registerStartAgentRun(mod *runtime.Module) {
	runtime.HandleCmd(mod, StartAgentRun, startAgentRunEffect)
}

func startAgentRunEffect(ctx context.Context, e Edges, p StartAgentRunPayload,
	emit runtime.Emit) error {
	// Resolve any requested secrets into the run environment at the edge (Flow C,
	// decision-004): the message carried only secret:// references, plaintext is
	// materialised here and nowhere else. Keyed by the request field name.
	var env map[string]string
	for field, url := range p.SecretRefs {
		plaintext, err := e.Secrets.Resolve(ctx, url)
		if err != nil {
			return err
		}
		if env == nil {
			env = make(map[string]string, len(p.SecretRefs))
		}
		env[field] = plaintext
	}

	// Mint the per-run notify token and inject the notification env vars, so an
	// agent can POST `kasi notify` to the host-gated control endpoint mid-run
	// (feature-notifications.md). The token is minted at the edge and recorded on
	// the AgentRun via record-notify-token BELOW — emitted before the harness even
	// starts, so the model holds the token before the agent could ever call notify.
	if env == nil {
		env = map[string]string{}
	}
	notifyToken := mintToken()
	env["KASI_TASK_ID"] = strconv.FormatInt(p.TaskID, 10)
	env["KASI_NOTIFY_TOKEN"] = notifyToken
	env["KASI_CONTROL_URL"] = e.ControlURL
	emit(NewRecordNotifyToken(RecordNotifyTokenPayload{TaskID: p.TaskID, RunID: p.RunID, Token: notifyToken}))

	// Provision every learned skill into this run's workspace before the harness
	// starts — the single choke point every run passes through, so skills authored
	// by any task are laid into every future run by default (Flow D, decision-009).
	if err := provisionSkills(e, p.TaskID); err != nil {
		return err
	}

	// Provision the whole memory collection into this run's in/ box beside the
	// skills — the same spawn choke point, so every run is handed käsi's durable
	// facts as ordinary files (in/memory/<name>.md + the in/MEMORY.md index) and its
	// provisioned name set is pinned for the harvest's deletion diff (feature-memory.md).
	mems := make([]workspace.MemoryFile, len(p.Memory))
	for i, m := range p.Memory {
		mems[i] = workspace.MemoryFile{Name: m.Name, Content: m.Content, Description: m.Description}
	}
	if err := e.Work.WriteMemory(p.TaskID, mems); err != nil {
		return err
	}

	// Advertise the running apps to this run as in/apps.json beside MEMORY.md — the
	// same choke point, so every run learns what it can call on localhost
	// (feature-apps.md). The URL is the app's LOCAL origin (the agent reaches it
	// inside the auth boundary); operations ride through as the app's raw app.json.
	appFiles := make([]workspace.AppFile, len(p.Apps))
	for i, a := range p.Apps {
		appFiles[i] = workspace.AppFile{Name: a.Name, URL: localAppURL(a.Port), Operations: []byte(a.Operations)}
	}
	if err := e.Work.WriteApps(p.TaskID, appFiles); err != nil {
		return err
	}

	// Symlink the persistent store into this run's workspace at ./store/ — the
	// same spawn choke point that provisions skills (Flow F, decision-012). The
	// store lives outside the workspace and the event log; the link makes it live
	// for the agent, and archival skips it so completing a task never touches it.
	if err := e.Store.Link(p.TaskID); err != nil {
		return err
	}

	// Empty out/ before the turn runs, so what the harvest finds afterwards is
	// exactly what THIS turn produced (decision-019). Without it a prior turn's
	// out/reply.txt lingers and a follow-up where the agent writes no new reply
	// re-sends the stale one — the "same email twice" bug. in/ is left intact so
	// prior context still accumulates for the agent.
	if err := e.Work.ResetOut(p.TaskID); err != nil {
		return err
	}

	// Register the live run and return immediately; the agent-watch
	// subscription emits finish-agent-run when the turn completes (docs/05).
	// The run is dispatched to its PINNED harness (decision-024), resolved by name
	// from the registry — the same harness across launch, watch, and signal, so a
	// restart resolves the one that launched.
	h := e.resolveHarness(p.Harness)
	// When the run is pinned to the REAL Codex harness, give this turn a private
	// CODEX_HOME (decision-025, the linchpin): a transient dir outside the workspace
	// and out/, seeded with a config.toml and the operator's credential materialized
	// from the reserved secret (or the host's, preserving today's posture). Only the
	// DIR PATH rides env — never the ~4KB blob, which would leak via /proc/<pid>/environ.
	// The Codex harness removes the home after the turn and writes back any token codex
	// rotated. Gated on the concrete *Codex, so every twin ring (sim/recorded/live
	// resolve a decorator, not *Codex) skips this entirely and its log and cassettes
	// stay byte-identical.
	var codexHome string
	if _, ok := h.(*Codex); ok {
		home, err := materializeCodexHome(ctx, e)
		if err != nil {
			return err
		}
		codexHome = home
		env["CODEX_HOME"] = home
	}
	var handle Handle
	var err error
	if p.Resume {
		session := p.Session
		if session == "" {
			session = sessionFor(p.TaskID)
		}
		handle, err = h.Resume(ctx, p.TaskID, p.RunID, session, env)
	} else {
		handle, err = h.Start(ctx, p.TaskID, p.RunID, env)
	}
	if err != nil {
		// The harness never registered a run, so its post-turn finalize will not fire;
		// remove the just-materialized CODEX_HOME here so the credential does not linger
		// (decision-025). Empty for every non-codex harness.
		if codexHome != "" {
			os.RemoveAll(codexHome)
		}
		return err
	}
	// Persist the session the harness returned, but ONLY when it minted its own —
	// i.e. it differs from the deterministic sessionFor (decision-024). Claude, the
	// sim, and the recorded twin all return sessionFor, so they never emit this and
	// their logs stay byte-identical and cassette-safe; only a harness like Codex
	// that mints a session id logs it, so the next turn's Resume reads it back.
	if handle.Session != "" && handle.Session != sessionFor(p.TaskID) {
		emit(NewRecordSession(RecordSessionPayload{TaskID: p.TaskID, RunID: p.RunID, Session: handle.Session}))
	}
	return nil
}

// provisionSkills lays every skill in the registry into the run's workspace
// skills/<name>/ box (docs/07 provisioning). It reads the skill table directly —
// an effect has no model — and unpacks each skill's tar tree, rooting it under the
// skill name. A corrupt blob is logged and skipped rather than blocking the run.
func provisionSkills(e Edges, taskID int64) error {
	rows, err := e.Content.AllSkills()
	if err != nil {
		return fmt.Errorf("agents: provision skills: %w", err)
	}
	for _, row := range rows {
		parts, err := skilltree.Unpack(row.Content)
		if err != nil {
			log.Printf("agents: provision skill %q: %v", row.Name, err)
			continue
		}
		rooted := make([]mime.Part, 0, len(parts))
		for _, part := range parts {
			part.Filename = row.Name + "/" + part.Filename
			rooted = append(rooted, part)
		}
		if err := e.Work.WriteSkills(taskID, rooted); err != nil {
			return fmt.Errorf("agents: provision skill %q: %w", row.Name, err)
		}
	}
	return nil
}
