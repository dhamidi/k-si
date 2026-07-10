// Package secrets is käsi's credential edge (docs/06): secrets are references
// (secret:// URLs) everywhere except at the moment of use, where a resolver —
// the single, auditable choke point — turns a URL into plaintext inside an
// effect. The plaintext is used and dropped; it never enters the model, the
// log, a message, or a workspace file.
//
// The real store (SQLiteSecrets) keeps values in a SEPARATE database file,
// encrypted at rest with a key held OUTSIDE that database (docs/06 invariant 5).
// The simulation ring wires SimSecrets, which hands out sentinel values so the
// scenario runner can scan for leaks (docs/13).
package secrets

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Secrets resolves a secret:// URL to its plaintext value, only at the instant
// an effect needs it (docs/06). Real and simulated implementations share this
// one interface; nothing outside an effect ever calls it.
type Secrets interface {
	Resolve(ctx context.Context, url string) (string, error)
}

// Entry is one stored secret as the management surface SEES it (docs/06): its
// reference and when it was last set — NEVER its value. It is what a /secrets
// list renders; the value stays sealed in the store and is only ever Resolved
// inside an effect (decision-004). UpdatedAt is zero when the edge does not track
// a set time (the sim twin).
type Entry struct {
	Ref       string
	UpdatedAt time.Time
}

const scheme = "secret"

// parseURL splits a secret://<namespace>/<key> reference into its namespace and
// key. It parses with net/url (scheme validation, percent-decoding, rejecting
// the malformed) and then treats host+path as one reference whose final segment
// is the key and whose remainder is the namespace — so a namespace may contain
// slashes (secret://route/pay/stripe-key → "route/pay", "stripe-key"), matching
// the namespace convention of docs/06.
func parseURL(raw string) (namespace, key string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("secrets: %q is not a valid URL: %w", raw, err)
	}
	if u.Scheme != scheme {
		return "", "", fmt.Errorf("secrets: %q is not a %s:// URL", raw, scheme)
	}

	ref := u.Host + u.Path
	i := strings.LastIndex(ref, "/")
	if i <= 0 || i == len(ref)-1 {
		return "", "", fmt.Errorf("secrets: malformed reference %q (want %s://<namespace>/<key>)", raw, scheme)
	}
	return ref[:i], ref[i+1:], nil
}

// URL builds the secret:// reference for a namespace and key (inputs we control,
// so plain composition; parseURL is where untrusted input is validated).
func URL(namespace, key string) string {
	return scheme + "://" + namespace + "/" + key
}
