package web

import "context"

// SimCodexSignIn is the simulated Codex sign-in launcher (decision-025, docs/13):
// the twin of the real device-auth launcher, used by scenarios so the whole
// sign-in loop runs without a real login. Start hands back a session with canned
// PUBLIC values (a fixed code and verification URL, safe to render) that reports
// waiting on the first poll and done on the next — so a scenario exercises the
// waiting-after-poll page before the credential is harvested. Faithful to the real
// twin, it does not hand the credential back through the poll: it calls the
// harvest callback with a marked, obviously-fake blob at the moment it "exits"
// (its second poll, modeling the subprocess ending), and the store discards it. It
// holds no real value, so a leak scan of the stored reference finds only the
// store's sentinel, never this blob (docs/06).
type SimCodexSignIn struct{}

// NewSimCodexSignIn builds the sim launcher the scenario web server wires through
// SetCodexSignIn.
func NewSimCodexSignIn() *SimCodexSignIn { return &SimCodexSignIn{} }

func (SimCodexSignIn) Start(ctx context.Context, harvest CodexHarvest) (CodexSignInSession, error) {
	return &simCodexSession{harvest: harvest}, nil
}

// simCodexSession is one canned sign-in. It reports waiting on the first poll and
// done on the next — enough for a scenario to see the waiting page (the code and
// URL), poll once and still be waiting, then reach the signed-in page on the
// second poll, exercising the waiting-after-poll branch. The second poll models
// the sign-in subprocess exiting: it harvests the sentinel credential through the
// callback exactly once (never returning it to the poll edge), mirroring the real
// twin's reap-time harvest.
type simCodexSession struct {
	harvest   CodexHarvest
	polled    bool
	harvested bool
	failed    bool
}

func (s *simCodexSession) Code() string            { return "KASI-CODEX-9F3A" }
func (s *simCodexSession) VerificationURL() string { return "https://auth.openai.com/device" }

func (s *simCodexSession) Poll() CodexSignInState {
	if !s.polled {
		s.polled = true
		return CodexSignInWaiting
	}
	// Model the subprocess exiting: harvest once, server-side, through the
	// callback. The blob — an obviously-fake sentinel — is handed straight to the
	// store and dropped; it is never rendered, logged, or returned here, so a
	// scenario can assert this marker appears nowhere on the page (decision-004).
	if !s.harvested {
		s.harvested = true
		if s.harvest != nil {
			if err := s.harvest([]byte(`{"note":"SIM-CODEX-CREDENTIAL-NEVER-RENDERED"}`)); err != nil {
				s.failed = true
			}
		}
	}
	if s.failed {
		return CodexSignInFailed
	}
	return CodexSignInDone
}

func (s *simCodexSession) Close() error { return nil }
