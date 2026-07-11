package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"
)

// CodexView is the data view_codex.vue renders — the /codex sign-in surface
// (decision-025): käsi signs in to Codex once, on the operator's behalf, and
// holds the result as a stored credential the Codex agent runs on. The page is
// host-gated, no token (decision-006), and shows exactly one of four states.
//
// "Signed in" is not a model field: it is derived from the secrets store — the
// reserved reference either exists or it does not (decision-004) — so this view
// carries only display strings and reverse-routed action paths, never a value.
// The one-time Code and VerificationURL shown while signing in are PUBLIC (the
// operator types them into their own browser); the credential itself never
// reaches this view.
type CodexView struct {
	// Exactly one of these is true, decided in Go so the template never compares
	// strings (the proven Nav/Request pattern):
	//   Disconnected — no credential and no sign-in under way; offer to sign in.
	//   Waiting      — a sign-in is under way; show the code and the URL to open.
	//   Connected    — the credential is stored; offer to sign out.
	//   Expired      — the last sign-in did not finish; offer to start again.
	Disconnected bool
	Waiting      bool
	Connected    bool
	Expired      bool

	// Code and VerificationURL are the one-time public values shown while a
	// sign-in is under way: open the URL, enter the code. Empty in every other
	// state. They are safe to render — they are not the credential (decision-004).
	Code            string
	VerificationURL string

	// ConnectPath starts (or restarts) a sign-in; PollPath re-checks one in
	// progress; DisconnectPath signs out (or cancels a sign-in under way);
	// SecretsPath links to the secrets list. All reverse-routed
	// (no-url-string-building).
	ConnectPath    string
	PollPath       string
	DisconnectPath string
	SecretsPath    string

	// Refresh is the meta-refresh directive the waiting page carries so it
	// re-checks the sign-in on its own without JavaScript (e.g. "5; url=/codex/connect").
	// Empty in every other state.
	Refresh string

	// Nav is the shared top-level navbar, with Settings lit — the section this
	// surface hangs off (navView).
	Nav NavView
}

// RenderCodex writes the full /codex page (docs/08).
func RenderCodex(ctx context.Context, w io.Writer, engine *htmlc.Engine, view CodexView) error {
	return engine.RenderPage(ctx, w, "view_codex", map[string]any{
		"codex": view,
	})
}
