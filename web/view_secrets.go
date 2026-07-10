package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"
)

// SecretsView is the data view_secrets.vue renders — the /secrets management
// surface (docs/06, decision-004): every stored secret as a REFERENCE with its
// last-set time (NEVER a value, no reveal control), an add/rotate form whose
// value field is masked and never echoes, a per-row delete, and a compact
// name-only audit trail. Built by the route handler from s.secrets.Entries plus
// credentials.Recent, never a raw model object (docs/08). Host-gated, no token
// (decision-006). A view never writes (docs/08); a value never enters it
// (decision-004).
type SecretsView struct {
	Secrets []SecretRow
	// Recent is the name-only audit trail (credentials.Recent), newest first —
	// reference + op ("set"/"removed") + time. It carries no value; it proves the
	// audit logging.
	Recent []AuditRow
	// Form is the add/rotate form object — empty on a plain GET, echoed with the
	// namespace/key and errors on an invalid submit. It NEVER carries the value.
	Form SecretsForm
	// SavePath is the POST target of the add/rotate form. Reverse-routed, never
	// string-built (rule no-url-string-building).
	SavePath string
	// Nav is the shared top-level navbar site_nav.vue renders (navView), this one lit.
	Nav NavView
}

// SecretRow is one stored secret in the list: its reference, the formatted
// last-set time ("—" when the edge tracks none, as the sim yields), and the
// reverse-routed confirm-delete target. NEVER a value.
type SecretRow struct {
	Ref        string
	UpdatedAt  string
	DeletePath string
}

// AuditRow is one entry in the name-only "Recent changes" trail: the reference,
// the operation ("set"/"removed"), and the formatted time. NEVER a value.
type AuditRow struct {
	Ref string
	Op  string
	At  string
}

// RenderSecrets writes the full /secrets page (docs/08).
func RenderSecrets(ctx context.Context, w io.Writer, engine *htmlc.Engine, view SecretsView) error {
	return engine.RenderPage(ctx, w, "view_secrets", map[string]any{
		"secrets": view,
	})
}

// ConfirmDeleteSecretView is the tiny no-JS safety page GET secrets.confirm-delete
// renders (decision-004): "Delete <ref>? [Confirm] [Cancel]", so a critical
// credential is not fat-fingered away. Confirm POSTs the delete with the reference
// in a hidden field; Cancel is a link back to the index. It carries only the
// reference (safe) — never a value.
type ConfirmDeleteSecretView struct {
	Ref        string
	DeletePath string
	CancelPath string
	Nav        NavView
}

// RenderConfirmDeleteSecret writes the confirm page (docs/08).
func RenderConfirmDeleteSecret(ctx context.Context, w io.Writer, engine *htmlc.Engine, view ConfirmDeleteSecretView) error {
	return engine.RenderPage(ctx, w, "view_confirm_delete_secret", map[string]any{
		"confirm": view,
	})
}
