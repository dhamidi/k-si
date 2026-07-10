package web

import (
	"log"
	"net/http"
	"regexp"
	"strings"

	credentialsmsg "github.com/dhamidi/k-si/credentials/msg"
	"github.com/dhamidi/k-si/secrets"
)

// secretNamespaceRE and secretKeyRE constrain the two reference parts so the
// built secret://<namespace>/<key> always parses (docs/06): a namespace may nest
// with slashes (secret://route/pay/stripe-key); a key is a single final segment,
// so it carries no slash. Both forbid whitespace (a space would break the URL
// parse) and must start with an alphanumeric. This is stricter than the edge's
// own parse, so a value that passes here can never fail secrets.URL round-trip.
var (
	secretNamespaceRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	secretKeyRE       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// SecretsForm — the add/rotate form object for the /secrets surface. It binds
// only the NAMESPACE and KEY: the reference parts, which are safe to echo on an
// invalid re-render. The plaintext VALUE is deliberately ABSENT — it is read
// straight off the POST in the handler, handed to the edge, and dropped, so it
// can never enter the form, the view, a message, the log, or a URL (decision-004).
type SecretsForm struct {
	Namespace string
	Key       string
	Errors    FormErrors
}

// BindSecretsForm reads the namespace and key (trimmed — a reference part carries
// no surrounding space). It pointedly does NOT read the value: a form object is
// re-rendered on an invalid submit, and a plaintext must never survive to be
// re-rendered (decision-004). Binding never fails — bad input becomes a field
// error in Validate.
func BindSecretsForm(r *http.Request) SecretsForm {
	return SecretsForm{
		Namespace: strings.TrimSpace(r.FormValue("namespace")),
		Key:       strings.TrimSpace(r.FormValue("key")),
		Errors:    FormErrors{},
	}
}

// Validate records a field error for a missing or malformed namespace/key. A
// valid namespace/key guarantees secrets.URL(ns, key) parses as a
// secret://<namespace>/<key> reference (first error per field wins).
func (f SecretsForm) Validate() SecretsForm {
	switch {
	case f.Namespace == "":
		f.Errors.Set("namespace", "a namespace is required")
	case !secretNamespaceRE.MatchString(f.Namespace):
		f.Errors.Set("namespace", "a namespace is letters, digits, dashes, dots, underscores, or slashes — no spaces")
	}
	switch {
	case f.Key == "":
		f.Errors.Set("key", "a key is required")
	case !secretKeyRE.MatchString(f.Key):
		f.Errors.Set("key", "a key is letters, digits, dashes, dots, underscores — no spaces or slashes")
	}
	return f
}

// Valid reports whether the reference may be built and the value stored.
func (f SecretsForm) Valid() bool { return len(f.Errors) == 0 }

// saveSecret is the write loop for an add/rotate (docs/08, decision-004): bind +
// validate the namespace/key; invalid re-renders the form (422) with the parts and
// errors preserved — but NEVER the value. Valid builds the reference, writes the
// plaintext at the edge, records the name-only audit event, and 303-redirects to
// the index. Setting an existing reference rotates it. The plaintext is read here,
// passed straight to Set, and dropped: it touches only that call — never the form,
// the view, the message, the log, or the URL.
func (s *Server) saveSecret(w http.ResponseWriter, r *http.Request) {
	form := BindSecretsForm(r).Validate()
	if !form.Valid() {
		s.renderSecrets(w, r, http.StatusUnprocessableEntity, form)
		return
	}

	ref := secrets.URL(form.Namespace, form.Key)

	// decision-004: the plaintext is read at the edge here, handed straight to the
	// store, and dropped — it exists only as this call's argument. If Set fails, log
	// the REFERENCE only, never the value.
	if err := s.secrets.Set(ref, r.FormValue("value")); err != nil {
		log.Printf("web: secrets: set failed for %s: %v", ref, err)
		http.Error(w, "could not store the secret", http.StatusInternalServerError)
		return
	}

	// The audit event carries ONLY the reference (decision-004). App.Send blocks
	// until applied, so the redirected GET shows the new trail and list.
	s.app.Send(credentialsmsg.NewRecordSecretSet(credentialsmsg.RecordSecretSetPayload{Ref: ref}))

	index, _ := s.router.Path("secrets.index", nil)
	http.Redirect(w, r, index, http.StatusSeeOther)
}

// deleteSecret is the write loop for a delete (docs/08, decision-004): it takes
// the reference from a hidden form field (a secret:// URL carries slashes and a
// scheme, so a hidden field is simpler and safer than encoding it in the path),
// removes it at the edge, records the name-only removal, and 303-redirects to the
// index. The reference is safe to carry; a value is never involved. Reaching this
// POST means the operator passed the confirm step (secrets.confirm-delete).
func (s *Server) deleteSecret(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ref := strings.TrimSpace(r.FormValue("ref"))
	if ref == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := s.secrets.Delete(ref); err != nil {
		log.Printf("web: secrets: delete failed for %s: %v", ref, err)
		http.Error(w, "could not delete the secret", http.StatusInternalServerError)
		return
	}

	// Name-only audit event (decision-004). App.Send blocks until applied.
	s.app.Send(credentialsmsg.NewRecordSecretRemoved(credentialsmsg.RecordSecretRemovedPayload{Ref: ref}))

	index, _ := s.router.Path("secrets.index", nil)
	http.Redirect(w, r, index, http.StatusSeeOther)
}
