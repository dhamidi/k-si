package web

import (
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/dhamidi/k-si/credentials"
	"github.com/dhamidi/k-si/secrets"
)

// recentSecretAudit is how many entries the /secrets "Recent changes" trail shows.
const recentSecretAudit = 20

// showSecrets renders the /secrets management page (docs/06, decision-004):
// every stored reference with its last-set time (NEVER a value), an add/rotate
// form, a per-row delete, and the name-only audit trail. Host-gated, no token
// (decision-006). The list reads references from the secrets edge (Entries) and
// the trail from the replayable credentials model (credentials.Recent) — a value
// is never read, so a value can never render.
func (s *Server) showSecrets(w http.ResponseWriter, r *http.Request) {
	s.renderSecrets(w, r, http.StatusOK, SecretsForm{Errors: FormErrors{}})
}

// renderSecrets writes the page from the current reference list plus a form object
// — empty on a plain GET, echoed with the namespace/key and errors on an invalid
// submit (the value is NEVER part of the form, so it can never be re-rendered).
func (s *Server) renderSecrets(w http.ResponseWriter, r *http.Request, status int, form SecretsForm) {
	entries, err := s.secrets.Entries()
	if err != nil {
		log.Printf("web: secrets: list references: %v", err)
		http.Error(w, "could not list secrets", http.StatusInternalServerError)
		return
	}

	view := SecretsView{
		Secrets:  s.secretRows(entries),
		Recent:   auditRows(credentials.Recent(s.app.View(), recentSecretAudit)),
		Form:     form,
		SavePath: s.secretsSavePath(),
		Nav:      s.navView("secrets.index"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := RenderSecrets(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render secrets: %v", err)
	}
}

// secretRows turns the reference list into rows — reference, formatted last-set
// time, and a reverse-routed confirm-delete target. NEVER a value: Entry carries
// only Ref and UpdatedAt (decision-004).
func (s *Server) secretRows(entries []secrets.Entry) []SecretRow {
	rows := make([]SecretRow, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, SecretRow{
			Ref:        e.Ref,
			UpdatedAt:  formatSecretTime(e.UpdatedAt),
			DeletePath: s.secretsConfirmDeletePath(e.Ref),
		})
	}
	return rows
}

// auditRows turns the name-only audit events into display rows — reference, op,
// and formatted time (decision-004: never a value).
func auditRows(events []credentials.Event) []AuditRow {
	rows := make([]AuditRow, 0, len(events))
	for _, e := range events {
		rows = append(rows, AuditRow{
			Ref: e.Ref,
			Op:  e.Op,
			At:  formatSecretTime(e.At),
		})
	}
	return rows
}

// formatSecretTime renders a set/change time, or "—" for a zero time (the sim
// edge tracks no clock, so its entries render the dash).
func formatSecretTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04")
}

// confirmDeleteSecret renders the no-JS safety step GET secrets.confirm-delete
// (decision-004): "Delete <ref>? [Confirm] [Cancel]". The reference rides the
// query (it is safe — never a value); Confirm POSTs the delete with it in a
// hidden field. Host-gated, no token (decision-006).
func (s *Server) confirmDeleteSecret(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	index, _ := s.router.Path("secrets.index", nil)
	view := ConfirmDeleteSecretView{
		Ref:        ref,
		DeletePath: s.secretsDeletePath(),
		CancelPath: index,
		Nav:        s.navView("secrets.index"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := RenderConfirmDeleteSecret(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render confirm-delete secret: %v", err)
	}
}

// secretsSavePath reverse-routes the add/rotate form's POST target (rule
// no-url-string-building).
func (s *Server) secretsSavePath() string {
	p, _ := s.router.Path("secrets.save", nil)
	return p
}

// secretsDeletePath reverse-routes the delete POST target.
func (s *Server) secretsDeletePath() string {
	p, _ := s.router.Path("secrets.delete", nil)
	return p
}

// secretsConfirmDeletePath reverse-routes a reference's confirm-delete GET target,
// carrying the reference as a query parameter through net/url (rule
// no-url-string-building) — the scheme-in-a-path problem a hidden field then
// finishes off on the POST.
func (s *Server) secretsConfirmDeletePath(ref string) string {
	p, _ := s.router.Path("secrets.confirm-delete", nil)
	u := url.URL{Path: p}
	q := u.Query()
	q.Set("ref", ref)
	u.RawQuery = q.Encode()
	return u.String()
}
