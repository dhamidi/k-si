package web

import (
	"context"
	"log"
	"net/http"
	"net/url"

	credentialsmsg "github.com/dhamidi/k-si/credentials/msg"
	"github.com/dhamidi/k-si/secrets"
)

// codexAuthRef is the reserved reference the Codex sign-in stores its credential
// under (decision-025). It is an ordinary decision-004 secret: the /secrets list
// renders it like any other reference, rotation is a re-Set, and "signed in" is
// defined as this reference being present in the store — no new model field. The
// namespace nests (codex/oauth) and the key is a single segment, so it parses as
// secret://codex/oauth/auth-json.
var codexAuthRef = secrets.URL("codex/oauth", "auth-json")

// CodexSignIn launches the host-gated Codex sign-in (decision-006, decision-025).
// The real twin shells the Codex sign-in against a dedicated käsi-managed home,
// captures the one-time public code and URL, and — the moment the sign-in
// subprocess exits successfully — harvests the credential and hands it to the
// harvest callback, all server-side, off any HTTP request. The sim twin returns
// canned public values and calls harvest with a sentinel credential, so scenarios
// exercise the whole loop without a real login. There is no inbound callback: the
// sign-in polls OpenAI out and the operator approves out-of-band. The credential
// NEVER rides an HTTP response or a poll GET — it flows only through harvest, at
// the process edge, then is dropped (decision-004).
type CodexSignIn interface {
	Start(ctx context.Context, harvest CodexHarvest) (CodexSignInSession, error)
}

// CodexHarvest stores the harvested credential at the moment the sign-in
// subprocess exits (decision-025). The blob is read at the process edge and handed
// straight to this callback, which puts it in the secrets store and drops it
// (decision-004) — it never reaches a view, a log, a message, or a URL. A non-nil
// error marks the sign-in failed, so Poll reports it and the operator retries.
type CodexHarvest func(authJSON []byte) error

// CodexSignInSession is one sign-in in progress. Code and VerificationURL are the
// PUBLIC one-time values shown to the operator (never the credential). Poll
// reports progress without blocking — CodexSignInDone means the subprocess exited
// and the harvest callback has already stored the credential (the poll edge only
// READS this outcome, it never touches the store); Close tears the attempt down
// and removes its transient home.
type CodexSignInSession interface {
	Code() string
	VerificationURL() string
	Poll() CodexSignInState
	Close() error
}

// CodexSignInState is where a sign-in stands: still waiting on the operator, done
// (the subprocess exited and the credential is already harvested and stored), or
// failed (the code expired, was declined, or the harvest could not be stored).
type CodexSignInState int

const (
	CodexSignInWaiting CodexSignInState = iota
	CodexSignInDone
	CodexSignInFailed
)

// SetCodexSignIn wires the sign-in launcher after construction (the real twin in
// production, the sim twin in scenarios), mirroring SetAppsOrigin so NewServer's
// signature stays put. Unset, the connect action reports the feature is
// unavailable rather than panicking.
func (s *Server) SetCodexSignIn(c CodexSignIn) { s.codexSignIn = c }

// showCodex renders the /codex sign-in page (decision-025). It derives the state
// from the secrets store and the one sign-in the server may be holding — never a
// model field, never a value.
func (s *Server) showCodex(w http.ResponseWriter, r *http.Request) {
	view := CodexView{
		ConnectPath:    s.codexConnectPath(),
		PollPath:       s.codexPollPath(),
		DisconnectPath: s.codexDisconnectPath(),
		SecretsPath:    s.secretsIndexPath(),
		Nav:            s.navView("settings.index"),
	}

	switch {
	case s.codexConnected():
		view.Connected = true
	case s.codexPending() != nil:
		pending := s.codexPending()
		view.Waiting = true
		view.Code = pending.Code()
		view.VerificationURL = pending.VerificationURL()
		// A meta-refresh re-checks the sign-in on its own, so the page settles to
		// "signed in" without JavaScript once the operator approves (docs/08).
		view.Refresh = "5; url=" + view.PollPath
	case r.URL.Query().Get("status") == "expired":
		view.Expired = true
	default:
		view.Disconnected = true
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := RenderCodex(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render codex: %v", err)
	}
}

// connectCodex starts a sign-in (decision-025): host-gated (decision-006), it asks
// the launcher to begin, holds the one running session, and redirects to /codex,
// which now shows the code and URL. Already signed in is a no-op redirect. At most
// one sign-in runs at a time — the operator signs in once — so the session needs
// no id in the URL, which keeps the whole flow reachable without client state.
func (s *Server) connectCodex(w http.ResponseWriter, r *http.Request) {
	if s.codexConnected() {
		http.Redirect(w, r, s.codexIndexPath(), http.StatusSeeOther)
		return
	}
	if s.codexSignIn == nil {
		http.Error(w, "Codex sign-in is not available.", http.StatusServiceUnavailable)
		return
	}

	// The harvest runs server-side when the sign-in subprocess exits, never on a
	// poll GET (decision-004, decision-025). It reads the credential at the process
	// edge and hands it straight to the store — the blob lives only as this Set's
	// argument, then is dropped: never a view, a log, a message, or a URL. The
	// name-only record-secret-set is the sole trace (decision-023). A store error
	// surfaces as a failed sign-in the operator retries.
	harvest := func(blob []byte) error {
		if err := s.secrets.Set(codexAuthRef, string(blob)); err != nil {
			log.Printf("web: codex: store credential for %s: %v", codexAuthRef, err)
			return err
		}
		s.app.Send(credentialsmsg.NewRecordSecretSet(credentialsmsg.RecordSecretSetPayload{Ref: codexAuthRef}))
		return nil
	}

	session, err := s.codexSignIn.Start(r.Context(), harvest)
	if err != nil {
		log.Printf("web: codex: start sign-in: %v", err)
		http.Error(w, "Could not start the Codex sign-in.", http.StatusInternalServerError)
		return
	}
	s.setCodexPending(session)

	http.Redirect(w, r, s.codexIndexPath(), http.StatusSeeOther)
}

// pollCodex re-checks the sign-in in progress (decision-025): the waiting page's
// meta-refresh and its "Check now" link both land here. This GET only READS where
// the sign-in stands — it never writes the secrets store. The credential is
// harvested and stored server-side the moment the sign-in subprocess exits (the
// harvest callback wired in connectCodex), so by the time Poll reports Done the
// store already holds it and this handler just returns to the signed-in page.
// Failed clears the attempt and marks it expired; still waiting falls back to
// /codex, which re-renders the code. It always redirects, and is idempotent: a
// second poll after the attempt is cleared finds it gone and returns to /codex.
func (s *Server) pollCodex(w http.ResponseWriter, r *http.Request) {
	session := s.codexPending()
	if session == nil {
		http.Redirect(w, r, s.codexIndexPath(), http.StatusSeeOther)
		return
	}

	switch session.Poll() {
	case CodexSignInDone:
		// The credential was already harvested and stored when the subprocess
		// exited; nothing to write here — drop the finished attempt and show the
		// signed-in page, which derives "signed in" from the stored reference.
		s.clearCodexPending()
		http.Redirect(w, r, s.codexIndexPath(), http.StatusSeeOther)
	case CodexSignInFailed:
		s.clearCodexPending()
		http.Redirect(w, r, s.codexExpiredPath(), http.StatusSeeOther)
	default:
		http.Redirect(w, r, s.codexIndexPath(), http.StatusSeeOther)
	}
}

// disconnectCodex signs out (decision-025): host-gated (decision-006), it cancels
// any sign-in under way, removes the reserved reference if one is stored, records
// the name-only removal (decision-004), and redirects to /codex. Signing out when
// not signed in is a harmless no-op — no stored reference, no recorded removal.
func (s *Server) disconnectCodex(w http.ResponseWriter, r *http.Request) {
	s.clearCodexPending()

	if s.codexConnected() {
		if err := s.secrets.Delete(codexAuthRef); err != nil {
			log.Printf("web: codex: remove credential for %s: %v", codexAuthRef, err)
			http.Error(w, "Could not sign out of Codex.", http.StatusInternalServerError)
			return
		}
		s.app.Send(credentialsmsg.NewRecordSecretRemoved(credentialsmsg.RecordSecretRemovedPayload{Ref: codexAuthRef}))
	}

	http.Redirect(w, r, s.codexIndexPath(), http.StatusSeeOther)
}

// codexConnected reports whether the reserved credential reference is stored —
// the sole definition of "signed in" (decision-025). It reads references only,
// never a value (decision-004). A store error degrades to "not signed in" rather
// than failing the page.
func (s *Server) codexConnected() bool {
	entries, err := s.secrets.Entries()
	if err != nil {
		log.Printf("web: codex: list references: %v", err)
		return false
	}
	for _, e := range entries {
		if e.Ref == codexAuthRef {
			return true
		}
	}
	return false
}

// codexPending returns the one sign-in the server is holding, or nil. Guarded so a
// production request racing another sees a consistent view.
func (s *Server) codexPending() CodexSignInSession {
	s.codexMu.Lock()
	defer s.codexMu.Unlock()
	return s.codexSession
}

// setCodexPending installs a fresh sign-in, closing any it replaces so a restarted
// sign-in never leaks the previous attempt's transient home.
func (s *Server) setCodexPending(session CodexSignInSession) {
	s.codexMu.Lock()
	prev := s.codexSession
	s.codexSession = session
	s.codexMu.Unlock()
	if prev != nil {
		if err := prev.Close(); err != nil {
			log.Printf("web: codex: close previous sign-in: %v", err)
		}
	}
}

// clearCodexPending drops and closes the held sign-in, if any (a harvest, a
// failure, or a sign-out).
func (s *Server) clearCodexPending() {
	s.codexMu.Lock()
	session := s.codexSession
	s.codexSession = nil
	s.codexMu.Unlock()
	if session != nil {
		if err := session.Close(); err != nil {
			log.Printf("web: codex: close sign-in: %v", err)
		}
	}
}

// codexIndexPath / codexConnectPath / codexPollPath / codexDisconnectPath reverse-
// route the /codex actions (no-url-string-building). codexExpiredPath is the index
// carrying the expired marker the poll redirects to on a failed sign-in.
func (s *Server) codexIndexPath() string {
	p, _ := s.router.Path("codex.index", nil)
	return p
}

func (s *Server) codexConnectPath() string {
	p, _ := s.router.Path("codex.connect", nil)
	return p
}

func (s *Server) codexPollPath() string {
	p, _ := s.router.Path("codex.poll", nil)
	return p
}

func (s *Server) codexDisconnectPath() string {
	p, _ := s.router.Path("codex.disconnect", nil)
	return p
}

func (s *Server) codexExpiredPath() string {
	p, _ := s.router.Path("codex.index", nil)
	u := url.URL{Path: p}
	q := u.Query()
	q.Set("status", "expired")
	u.RawQuery = q.Encode()
	return u.String()
}

// secretsIndexPath reverse-routes the /secrets list (the connected state links to
// it — the reserved reference shows there like any other).
func (s *Server) secretsIndexPath() string {
	p, _ := s.router.Path("secrets.index", nil)
	return p
}
