package agents

import (
	"crypto/rand"
	"encoding/base64"
)

// mintToken mints an unguessable per-run capability token — 128 bits of
// crypto/rand, URL-safe. Randomness enters here at the edge (start-agent-run),
// never in a pure handler; the minted value rides record-notify-token into the log
// (docs/13), mirroring the completion token minted at the inbound edge in serve.go.
func mintToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is fatal for a secret; don't fall back to anything
		// guessable.
		panic("agents: start-agent-run: crypto/rand: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
