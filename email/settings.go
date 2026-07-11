package email

import (
	"fmt"
	"net/mail"
	"net/url"
	"strings"

	"github.com/dhamidi/k-si/admin"
	"github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/settings"
	"github.com/dhamidi/k-si/tasks"
)

// Allowlist is the initiator allowlist as a setting VALUE (docs/16,
// decision-020): a named []string (introduced here so the slice can carry the
// form methods) whose state stays in email.Model.Allowlist. It is the DYNAMIC
// setting — a list whose shape grows and shrinks as you edit it — so it
// implements ToForm explicitly to carry an Update, and parses each row through
// addressValue, the parse-don't-validate mechanism at the element level.
type Allowlist []string

// AllowlistOf reads the initiator allowlist out of the model — the pure View
// read the settings surface renders and writes through (docs/15).
func AllowlistOf(v runtime.View) Allowlist {
	return Allowlist(runtime.Slice[Model](v, "email").Allowlist)
}

// addressValue is a flag.Value validating one email address — the element-level
// parse-don't-validate contract each allowlist row runs through. An empty string
// is allowed (a blank, just-added row parses to "nothing" and is dropped), so the
// operator can add a row and fill it later without a spurious error.
type addressValue string

func (a *addressValue) Set(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		*a = ""
		return nil
	}
	if _, err := mail.ParseAddress(raw); err != nil {
		return fmt.Errorf("must be a deliverable email address, e.g. alice@decode.ee")
	}
	*a = addressValue(raw)
	return nil
}

func (a addressValue) String() string { return string(a) }

// rowFields is one KindText control per address — the form's shape mirrors the
// list's current contents, so a fresh render shows every address in its own row.
func rowFields(a Allowlist) []settings.Field {
	fields := make([]settings.Field, 0, len(a))
	for i, addr := range a {
		fields = append(fields, settings.Field{
			Name:  addrName(i),
			Label: "Address",
			Kind:  settings.KindText,
			Value: addr,
		})
	}
	return fields
}

func addrName(i int) string { return fmt.Sprintf("addr.%d", i) }

// nextAddrIndex returns the next free row name index — one past the highest
// existing addr.N. It does NOT renumber the surviving rows on a remove: a row's
// name is its stable identity across a reshape round-trip, so the submitted
// values (keyed by name) re-bind onto the exact rows they belong to and a
// removed row's neighbours keep their values (docs/16). A fresh, contiguous
// numbering happens naturally on the next full render from the model.
func nextAddrIndex(fields []settings.Field) int {
	max := -1
	for _, f := range fields {
		var i int
		if _, err := fmt.Sscanf(f.Name, "addr.%d", &i); err == nil && i > max {
			max = i
		}
	}
	return max + 1
}

// ToForm builds the allowlist's dynamic form (docs/16): one text row per address,
// an Update that grows/shrinks the row set, and a Parse that turns the filled
// rows back into a validated Allowlist. The list is a list of NON-sensitive
// fields, so the whole reshape body re-renders freely with no secret ever echoed
// (decision-004).
func (a Allowlist) ToForm() settings.Form {
	f := settings.Form{Fields: rowFields(a)}

	f.Update = func(f settings.Form, ev settings.Event) settings.Form {
		switch ev.Op {
		case "add":
			f.Fields = append(f.Fields, settings.Field{
				Name:  addrName(nextAddrIndex(f.Fields)),
				Label: "Address",
				Kind:  settings.KindText,
			})
		case "remove":
			if ev.Index >= 0 && ev.Index < len(f.Fields) {
				f.Fields = append(f.Fields[:ev.Index], f.Fields[ev.Index+1:]...)
			}
		}
		return f
	}

	f.Parse = func(f settings.Form) (settings.Value, settings.FieldErrors) {
		out, errs := Allowlist{}, settings.FieldErrors{}
		for _, fld := range f.Fields {
			var addr addressValue // Set validates one address; "" drops the row
			if err := addr.Set(fld.Value); err != nil {
				errs.Set(fld.Name, err.Error())
			} else if addr != "" {
				out = append(out, string(addr))
			}
		}
		return out, errs
	}

	return f
}

// forwardMechanism is the ForwardEmail delivery mechanism as a setting VALUE
// (decision-023): a flat, multi-field setting whose state lives in
// email.Model.Mechanisms["forwardemail"] and email.Model.OutboundVia. Unlike the
// dynamic allowlist it never reshapes, so its ToForm carries no Update — a fixed
// set of top-level fields. The credential fields hold secret:// references, never
// plaintext: the token and password live in the secrets store and the form only
// ever names them (decision-004). active and deliverable are read from the model
// so Parse and Write can decide correctly without a second view read.
type forwardMechanism struct {
	Domain      string
	SendCredRef string
	RecvCredRef string
	Inbound     bool
	Outbound    bool
	active      bool // forwardemail is currently the active sender (OutboundVia)
	deliverable bool // a real reply-from + base URL exist right now
}

func boolField(b bool) string {
	if b {
		return settings.True
	}
	return settings.False
}

// The conventional secret references ForwardEmail's credentials live at. The
// outbound sender is built at boot to resolve its token from the first of these
// (cmd/kasi/serve.go), so the send credential must be stored there; the form
// pre-fills both as a guide, and the operator stores the matching secrets on the
// Secrets page (decision-023, decision-004).
const (
	forwardEmailTokenRef = "secret://forwardemail/api-token"
	forwardEmailIMAPRef  = "secret://forwardemail/imap-password"
)

// orDefault fills an empty reference with its conventional path, so an
// unconfigured form shows the operator exactly where to store each credential.
func orDefault(ref, def string) string {
	if ref == "" {
		return def
	}
	return ref
}

// ToForm builds the flat ForwardEmail form: the domain, the two credential
// references, and the receive/send toggles, plus a Parse that reads them back and
// enforces the mechanism's rules. No Update — the shape is fixed (decision-023).
func (m forwardMechanism) ToForm() settings.Form {
	f := settings.Form{Fields: []settings.Field{
		{Name: "domain", Label: "Your sending domain", Kind: settings.KindText, Value: m.Domain},
		{Name: "send_cred", Label: "API token — secret reference", Kind: settings.KindText, Value: m.SendCredRef},
		{Name: "recv_cred", Label: "IMAP password — secret reference", Kind: settings.KindText, Value: m.RecvCredRef},
		{Name: "inbound", Label: "Receive mail through ForwardEmail", Kind: settings.KindBool, Value: boolField(m.Inbound)},
		{Name: "outbound", Label: "Send replies through ForwardEmail", Kind: settings.KindBool, Value: boolField(m.Outbound)},
	}}

	f.Parse = func(f settings.Form) (settings.Value, settings.FieldErrors) {
		out := forwardMechanism{active: m.active, deliverable: m.deliverable}
		errs := settings.FieldErrors{}
		for _, fld := range f.Fields {
			switch fld.Name {
			case "domain":
				out.Domain = strings.TrimSpace(fld.Value)
			case "send_cred":
				out.SendCredRef = strings.TrimSpace(fld.Value)
			case "recv_cred":
				out.RecvCredRef = strings.TrimSpace(fld.Value)
			case "inbound":
				out.Inbound = fld.Value == settings.True
			case "outbound":
				out.Outbound = fld.Value == settings.True
			}
		}

		// Credentials are references, never plaintext (decision-004): a value here
		// must be a secret:// reference or empty, so a token typed into this box can
		// never reach the model or the log.
		if out.SendCredRef != "" && !strings.HasPrefix(out.SendCredRef, "secret://") {
			errs.Set("send_cred", "must be a secret reference like secret://forwardemail/api-token — store the token on the Secrets page, then enter that reference here")
		}
		if out.RecvCredRef != "" && !strings.HasPrefix(out.RecvCredRef, "secret://") {
			errs.Set("recv_cred", "must be a secret reference like secret://forwardemail/imap-password — store the password on the Secrets page, then enter that reference here")
		}
		// A provider cannot receive without its IMAP password, or send without its
		// API token: refuse to switch a direction on with no credential behind it.
		if out.Inbound && out.RecvCredRef == "" {
			errs.Set("recv_cred", "receiving needs the IMAP password's secret reference")
		}
		if out.Outbound && out.SendCredRef == "" {
			errs.Set("send_cred", "sending needs the API token's secret reference")
		}
		// Turning the sender on needs a deliverable identity — a real reply-from
		// address and public base URL, checked when the form was read — so a switch
		// can never start sending mail nobody receives (decision-023).
		if out.Outbound && !out.deliverable {
			errs.Set("outbound", "set a deliverable reply-from address and public base URL first, or replies would be undeliverable")
		}
		return out, errs
	}

	return f
}

// deliverableIdentity reports whether käsi could deliver outbound mail right now: a
// reply-from on a real (non-.test) domain, and a base URL that resolves for
// recipients. It is the same check the -send boot guard performs
// (cmd/kasi/serve.go), moved to the outbound-enable path so enabling a sender in
// the UI is refused unless mail could actually be delivered (decision-023).
func deliverableIdentity(v runtime.View) bool {
	from := tasks.ReplyFrom(v)
	if from == "" || strings.HasSuffix(mime.Domain(from), ".test") {
		return false
	}
	u, err := url.Parse(admin.BaseURLOf(v).String())
	if err != nil || u.Hostname() == "" || strings.HasSuffix(u.Hostname(), ".test") {
		return false
	}
	return true
}

// Settings is email's contribution to the settings surface (docs/16): the initiator
// allowlist (the DYNAMIC reshape setting) and the ForwardEmail mechanism (the flat,
// secret-bearing setting, decision-023). Both keep their state in email.Model; each
// is a read plus a whole-value write over it, not a relocation.
func Settings() []settings.Setting {
	return []settings.Setting{
		{
			Key:   "initiators",
			Short: "Initiator allowlist",
			Long:  "The email addresses allowed to start new tasks. Anyone listed here may open a task by email; everyone else is ignored. Add or remove rows, then Save.",
			Owner: "email",
			Read:  func(v runtime.View) settings.Value { return AllowlistOf(v) },
			Write: func(val settings.Value) []runtime.Msg {
				return []runtime.Msg{msg.NewSetAllowlist(msg.SetAllowlistPayload{Addresses: []string(val.(Allowlist))})}
			},
		},
		{
			Key:   "forwardemail",
			Short: "ForwardEmail delivery",
			Long:  "Send and receive mail through ForwardEmail. Store your API token and IMAP password on the Secrets page, enter their references here, then turn receiving and sending on. Sending signs each reply with DKIM on your domain so it lands in the inbox.",
			Owner: "email",
			Read: func(v runtime.View) settings.Value {
				mech, _ := MechanismOf(v, "forwardemail")
				return forwardMechanism{
					Domain:      mech.Domain,
					SendCredRef: orDefault(mech.SendCredRef, forwardEmailTokenRef),
					RecvCredRef: orDefault(mech.RecvCredRef, forwardEmailIMAPRef),
					Inbound:     mech.Inbound,
					Outbound:    mech.Outbound,
					active:      OutboundVia(v) == "forwardemail",
					deliverable: deliverableIdentity(v),
				}
			},
			Write: func(val settings.Value) []runtime.Msg {
				m := val.(forwardMechanism)
				msgs := []runtime.Msg{msg.NewSetMechanism(msg.SetMechanismPayload{
					Name:        "forwardemail",
					Inbound:     m.Inbound,
					Outbound:    m.Outbound,
					Domain:      m.Domain,
					SendCredRef: m.SendCredRef,
					RecvCredRef: m.RecvCredRef,
				})}
				// Enabling outbound makes ForwardEmail the active sender; disabling it
				// when it WAS the active sender falls back to the spool, so replies stop
				// leaving through a provider that was just turned off (decision-023).
				if m.Outbound {
					msgs = append(msgs, msg.NewSetOutboundVia(msg.SetOutboundViaPayload{Name: "forwardemail"}))
				} else if m.active {
					msgs = append(msgs, msg.NewSetOutboundVia(msg.SetOutboundViaPayload{Name: "spool"}))
				}
				return msgs
			},
		},
	}
}
