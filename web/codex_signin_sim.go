package web

import "context"

// SimCodexSignIn is the simulated Codex sign-in launcher (decision-025, docs/13):
// the twin of the real device-auth launcher, used by scenarios so the whole
// sign-in loop runs without a real login. Start hands back a session with canned
// PUBLIC values (a fixed code and verification URL, safe to render) that reports
// waiting on the first poll and done on the next — so a scenario exercises the
// waiting-after-poll page before the credential is harvested — and yields a
// marked, obviously-fake credential the web edge stores and drops. It holds no
// real value, so a leak scan of the stored reference finds only the store's
// sentinel, never this blob (docs/06).
type SimCodexSignIn struct{}

// NewSimCodexSignIn builds the sim launcher the scenario web server wires through
// SetCodexSignIn.
func NewSimCodexSignIn() *SimCodexSignIn { return &SimCodexSignIn{} }

func (SimCodexSignIn) Start(ctx context.Context) (CodexSignInSession, error) {
	return &simCodexSession{}, nil
}

// simCodexSession is one canned sign-in. It reports waiting on the first poll and
// done on the next — enough for a scenario to see the waiting page (the code and
// URL), poll once and still be waiting, then reach the signed-in page on the
// second poll, exercising the waiting-after-poll branch.
type simCodexSession struct {
	polled bool
}

func (s *simCodexSession) Code() string            { return "KASI-CODEX-9F3A" }
func (s *simCodexSession) VerificationURL() string { return "https://auth.openai.com/device" }

func (s *simCodexSession) Poll() CodexSignInState {
	if !s.polled {
		s.polled = true
		return CodexSignInWaiting
	}
	return CodexSignInDone
}

// AuthJSON is the obviously-fake credential the sim yields on a successful
// harvest. The web edge hands it straight to the store, which discards it; it is
// never rendered or logged, so a scenario can assert this marker appears nowhere
// on the page (decision-004).
func (s *simCodexSession) AuthJSON() ([]byte, error) {
	return []byte(`{"note":"SIM-CODEX-CREDENTIAL-NEVER-RENDERED"}`), nil
}

func (s *simCodexSession) Close() error { return nil }
