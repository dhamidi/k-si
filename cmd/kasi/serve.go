package main

// `kasi serve` — the running system (docs/01). It assembles the same modules as
// the test runner, but wired to REAL edges: SQLite for the log and content
// tables, a separate secrets database, on-disk workspaces, the Claude harness,
// and — because only a read-only Fastmail token exists — a spool directory as
// the outbound sender. Inbound polling of Fastmail is opt-in (`-poll`): it routes
// real mail into real agent runs, so it is off by default.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhamidi/k-si/admin"
	adminmsg "github.com/dhamidi/k-si/admin/msg"
	"github.com/dhamidi/k-si/agents"
	agentmsg "github.com/dhamidi/k-si/agents/msg"
	"github.com/dhamidi/k-si/apprunner"
	"github.com/dhamidi/k-si/apps"
	"github.com/dhamidi/k-si/counter"
	"github.com/dhamidi/k-si/datastore"
	"github.com/dhamidi/k-si/email"
	emailmsg "github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/memory"
	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/secrets"
	"github.com/dhamidi/k-si/skills"
	"github.com/dhamidi/k-si/store"
	"github.com/dhamidi/k-si/tasks"
	taskmsg "github.com/dhamidi/k-si/tasks/msg"
	"github.com/dhamidi/k-si/web"
	"github.com/dhamidi/k-si/workspace"
)

func runServe(args []string) int {
	flags := flag.NewFlagSet("kasi serve", flag.ExitOnError)
	addr := flags.String("addr", "127.0.0.1:8787", "listen address (host-gated deployment, docs/08)")
	state := flags.String("state", "data", "state directory holding the databases (docs/03)")
	workdir := flags.String("workdir", "data/work", "task workspaces ($WORKDIR, docs/05)")
	spooldir := flags.String("spool", "data/spool", "outbound mail spool (.eml files) — used unless -send is set")
	// ast-grep-ignore: no-placeholder-domain  flag DEFAULT only; real -send rejects a .test base-url in the guard below
	baseURL := flags.String("base-url", "https://kasi.test", "public base URL for capability links (docs/04)")
	allow := flags.String("allow", "", "comma-separated addresses to seed the initiator allowlist (docs/04)")
	poll := flags.Bool("poll", false, "poll Fastmail for inbound mail — routes REAL mail into agent runs (off by default)")
	send := flags.Bool("send", false, "submit replies through Fastmail — sends REAL mail (off by default; spools otherwise)")
	from := flags.String("from", "", "deliverable From address replies are sent as (an address you can send for, e.g. kasi@decode.ee)")
	// Runaway breakers (SEV1 self-reply loop, decision-016). Defaults are ON in
	// production: this box realistically runs 1–2 concurrent claude processes, and a
	// single task rarely needs more than ~20 turns, so these bound a loop's blast
	// radius without touching normal use. 0 disables either guard.
	maxConcurrent := flags.Int("max-concurrent-runs", 2, "cap live agent runs; the rest queue (0 = unlimited) — OOM breaker (docs/decision-016)")
	maxTaskRuns := flags.Int("max-task-runs", 20, "pause a task after this many agent runs without resolving (0 = off) — loop breaker (docs/decision-016)")
	flags.Parse(args)

	// Real send must use a deliverable From and a reachable link origin — a
	// .test domain has no SPF/DKIM/MX and no DNS, so recipients would spam-file
	// or drop the reply. Refuse rather than send mail nobody receives.
	if *send {
		if *from == "" || strings.HasSuffix(mime.Domain(*from), ".test") {
			return fail("kasi serve:", fmt.Errorf("-send needs a real -from (an address on a domain you can send for); %q won't deliver", *from))
		}
		if u, err := url.Parse(*baseURL); err != nil || u.Hostname() == "" || strings.HasSuffix(u.Hostname(), ".test") {
			return fail("kasi serve:", fmt.Errorf("-send needs a real -base-url; %q won't resolve for recipients", *baseURL))
		}
	}

	if err := os.MkdirAll(*workdir, 0o755); err != nil {
		return fail("kasi serve:", err)
	}

	logStore, err := store.OpenSQLiteLog(filepath.Join(*state, "kasi.db"))
	if err != nil {
		return fail("kasi serve:", err)
	}
	defer logStore.Close()
	content, err := store.OpenSQLiteContent(filepath.Join(*state, "content.db"))
	if err != nil {
		return fail("kasi serve:", err)
	}
	defer content.Close()
	key, err := secrets.LoadKey(*state)
	if err != nil {
		return fail("kasi serve:", err)
	}
	sec, err := secrets.OpenSQLite(filepath.Join(*state, "secrets.db"), key)
	if err != nil {
		return fail("kasi serve:", err)
	}
	defer sec.Close()

	clock := runtime.RealClock{}
	work := workspace.NewOS(*workdir)
	// The agent's persistent store: one directory under $STATE, symlinked into
	// every run's workspace at ./store/ and persisting across tasks — outside the
	// event log, like the mail edge (Flow F, decision-012). It lives beside the
	// workspace root (*workdir) so completing a task never touches it.
	dataStore, err := datastore.NewOS(filepath.Join(*state, "store"), *workdir)
	if err != nil {
		return fail("kasi serve:", err)
	}
	// The app runner keeps registered apps up under systemd --user (feature-apps.md).
	// Apps live as direct children of the store, so its root is the same store dir.
	appRunner := apprunner.NewOS(filepath.Join(*state, "store"))
	// One JMAP client serves both real-world directions. Outbound defaults to the
	// spool sender — replies written to <spool>/*.eml for inspection — while -send
	// submits them through Fastmail for real. Sending real mail is outward-facing,
	// so it is opt-in like -poll; a production deployment runs with both (docs/04).
	jmap := email.NewJMAP(sec, "secret://fastmail/api-token")
	var outbound email.Mail = email.NewSpoolMail(*spooldir)
	if *send {
		outbound = jmap
	}

	app := runtime.New(
		admin.Module(admin.Edges{Clock: clock}),
		apps.Module(apps.Edges{Clock: clock, Runner: appRunner}),
		memory.Module(memory.Edges{Clock: clock}),
		skills.Module(skills.Edges{Clock: clock}),
		counter.Module(counter.Edges{Clock: clock}),
		email.Module(email.Edges{Clock: clock, Mail: outbound, Content: content, Work: work}),
		tasks.Module(tasks.Edges{Clock: clock, Work: work, Content: content}),
		agents.Module(agents.Edges{Store: dataStore, Clock: clock, Harness: agents.NewClaude(*workdir), Work: work, Secrets: sec, Content: content, ControlURL: controlURL(*addr)}),
	).UseLog(logStore).UseClock(clock)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := app.Start(ctx); err != nil {
		return fail("kasi serve:", err)
	}
	defer app.Stop()

	// Seed the reply-from identity ONLY when unset, so a UI edit survives a restart
	// and a re-passed -from never clobbers it — the guarded seeding that makes the
	// setting "editable thereafter" true (docs/16, decision-020).
	if *from != "" && tasks.ReplyFrom(app.View()) == "" {
		app.Send(taskmsg.NewSetReplyFrom(taskmsg.SetReplyFromPayload{Address: *from}))
	}
	// Arm the runaway breakers (decision-016). These stay UNCONDITIONAL: the int caps
	// default to non-zero (2, 20) and 0 is a legitimate value ("disabled"/"unlimited"),
	// so the model carries no clean "unset" signal to guard on, and a wrong guard is
	// worse than a re-seed. Guarding them needs a "has this been set" flag in the model.
	// TODO(phase 3): give the caps a guarded seed (a sentinel or a set flag) so a UI
	// edit to a cap also survives a restart, like reply-from and base-url now do.
	app.Send(agentmsg.NewSetMaxConcurrentRuns(agentmsg.SetMaxConcurrentRunsPayload{Max: *maxConcurrent}))
	app.Send(taskmsg.NewSetLoopGuard(taskmsg.SetLoopGuardPayload{Max: *maxTaskRuns}))
	seedAllowlist(app, *allow)
	// Seed the public base URL into admin's model ONLY when unset, so a UI edit
	// survives a restart and a defaulted -base-url never re-seeds over it — the
	// migration from a boot-frozen edge to logged, editable state (docs/16,
	// decision-020). The flag still parses/validates at boot (the -send guard above).
	if *baseURL != "" && admin.BaseURLOf(app.View()) == "" {
		app.Send(adminmsg.NewSetBaseURL(adminmsg.SetBaseURLPayload{URL: *baseURL}))
	}

	if *poll {
		go pollInbox(ctx, app, jmap, content)
	}

	// The /apps page reads each app's liveness and logs through the same runner
	// that keeps them up (feature-apps.md). The settings surface renders and writes
	// the typed contributions, assembled in the open beside the module list (docs/16,
	// decision-020). email.Settings() contributes the DYNAMIC initiator allowlist,
	// which the reshape round-trip and content-negotiated Turbo drive (phase 3).
	server, err := web.NewServer(app, sec, content, work, appRunner, dataStore, web.Settings(
		admin.Settings(), email.Settings(), tasks.Settings(), agents.Settings(),
	))
	if err != nil {
		return fail("kasi serve:", err)
	}
	// Apps are addressed under the public origin (scheme+host, no port) so the
	// control endpoint mints public-correct URLs; the per-app port is appended
	// (feature-apps.md). Derived from -base-url, dropping any port it carries.
	if *baseURL != "" {
		if u, err := url.Parse(*baseURL); err == nil && u.Hostname() != "" {
			server.SetAppsOrigin(u.Scheme + "://" + u.Hostname())
		}
	}
	httpServer := &http.Server{Addr: *addr, Handler: server}
	go func() {
		<-ctx.Done()
		httpServer.Close()
	}()

	outboundDesc := "spool=" + *spooldir
	if *send {
		outboundDesc = "send=fastmail"
	}
	fmt.Printf("kasi: http://%s  state=%s  %s  poll=%v  (Ctrl-C to stop)\n", *addr, *state, outboundDesc, *poll)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fail("kasi serve:", err)
	}
	return 0
}

// seedAllowlist adds the -allow addresses to the initiator allowlist, skipping
// any already present so a restart does not re-log them (docs/04).
func seedAllowlist(app *runtime.App, csv string) {
	for _, addr := range strings.Split(csv, ",") {
		addr = strings.TrimSpace(addr)
		if addr == "" || email.IsAllowed(app.View(), addr) {
			continue
		}
		app.Send(emailmsg.NewAllowSender(emailmsg.AllowSenderPayload{Address: addr}))
	}
}

// pollInbox is the inbound edge: it polls Fastmail for new mail and injects a
// route-email for each, exactly as a subscription would (docs/01: an edge changes
// state only by putting a message on the channel). It routes REAL mail, which is
// why it is opt-in.
//
// The high-water mark lives in the LOG, not in this goroutine: it is seeded from
// the replayed model and advanced only through record-poll-state. So a restart
// resumes from the last-processed state and Email/changes hands back the mail that
// arrived while käsi was offline, instead of the goroutine re-anchoring to "now"
// and skipping the whole downtime window (offline-gap fix, decision-018). A first
// deployment has an empty cursor, so Fetch("") anchors to now and logs that anchor.
func pollInbox(ctx context.Context, app *runtime.App, jmap *email.JMAP, content *store.SQLiteContent) {
	state := email.PollCursor(app.View())
	for {
		msgs, next, err := jmap.Fetch(ctx, state)
		if err != nil {
			log.Printf("poll: %v", err)
		} else {
			for _, m := range msgs {
				route(app, content, m)
			}
			// Advance the cursor through the log AFTER the batch is routed. A crash
			// in the gap replays the old cursor and re-Fetches the batch, which
			// route-email absorbs (idempotent on an already-ingested inbox row,
			// decision-018) — the safe direction. Only log a genuine advance so an
			// idle poll doesn't append a redundant entry every 15s.
			if next != state {
				app.Send(email.NewRecordPollState(email.RecordPollStatePayload{State: next}))
				state = next
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(15 * time.Second):
		}
	}
}

func route(app *runtime.App, content *store.SQLiteContent, m email.Inbound) {
	id, err := content.AddInbox(store.InboxRow{MessageID: m.MessageID, Recipient: m.Recipient, Raw: m.Raw, Status: "new"})
	if err != nil {
		log.Printf("poll: inbox: %v", err)
		return
	}
	parsed, err := mime.Parse(m.Raw)
	if err != nil {
		log.Printf("poll: parse: %v", err)
		return
	}
	app.Send(email.NewRouteEmail(email.RouteEmailPayload{
		InboxID:         id,
		Recipient:       m.Recipient,
		Sender:          firstAddr(parsed.Header.Get("From")),
		To:              mime.CcList(parsed.Header.Get("To")),
		Cc:              mime.CcList(parsed.Header.Get("Cc")),
		Subject:         parsed.Header.Get("Subject"),
		MessageID:       m.MessageID,
		InReplyTo:       parsed.Header.Get("In-Reply-To"),
		References:      strings.Fields(parsed.Header.Get("References")),
		CompletionToken: mintToken(),
	}))
}

// mintToken mints an unguessable completion token at the inbound edge — 128 bits
// of crypto/rand, URL-safe. Randomness enters here, not in a pure handler, and
// rides route-email into the log as a recorded value (docs/13).
func mintToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is fatal for a secret; don't fall back to anything
		// guessable.
		panic("kasi serve: crypto/rand: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// controlURL derives the loopback origin an in-run agent POSTs notifications to
// from the server's listen address (feature-notifications.md). A wildcard host
// (empty, 0.0.0.0, or ::) is rewritten to 127.0.0.1 so the agent reaches the
// server over loopback; a real host is kept. On a parse failure it falls back to
// the address verbatim.
func controlURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return (&url.URL{Scheme: "http", Host: addr}).String()
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return (&url.URL{Scheme: "http", Host: net.JoinHostPort(host, port)}).String()
}

func firstAddr(header string) string {
	if addrs := mime.CcList(header); len(addrs) > 0 {
		return addrs[0]
	}
	return ""
}

func fail(prefix string, err error) int {
	fmt.Fprintln(os.Stderr, prefix, err)
	return 1
}
