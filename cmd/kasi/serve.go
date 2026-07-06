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
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhamidi/k-si/agents"
	"github.com/dhamidi/k-si/counter"
	"github.com/dhamidi/k-si/email"
	emailmsg "github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/secrets"
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
		counter.Module(counter.Edges{Clock: clock}),
		email.Module(email.Edges{Clock: clock, Mail: outbound, Content: content, Work: work, BaseURL: *baseURL}),
		tasks.Module(tasks.Edges{Clock: clock, Work: work, Content: content}),
		agents.Module(agents.Edges{Clock: clock, Harness: agents.NewClaude(*workdir), Work: work}),
	).UseLog(logStore).UseClock(clock)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := app.Start(ctx); err != nil {
		return fail("kasi serve:", err)
	}
	defer app.Stop()

	if *from != "" {
		app.Send(taskmsg.NewSetReplyFrom(taskmsg.SetReplyFromPayload{Address: *from}))
	}
	seedAllowlist(app, *allow)

	if *poll {
		go pollInbox(ctx, app, jmap, content)
	}

	server, err := web.NewServer(app)
	if err != nil {
		return fail("kasi serve:", err)
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
// state only by putting a message on the channel). The high-water state starts at
// "now", so only mail arriving after serve starts is processed. It routes REAL
// mail, which is why it is opt-in.
func pollInbox(ctx context.Context, app *runtime.App, jmap *email.JMAP, content *store.SQLiteContent) {
	state := ""
	for {
		msgs, next, err := jmap.Fetch(ctx, state)
		if err != nil {
			log.Printf("poll: %v", err)
		} else {
			state = next
			for _, m := range msgs {
				route(app, content, m)
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
