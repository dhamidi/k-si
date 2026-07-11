package email

import (
	"fmt"
	"net/mail"
	"strings"

	"github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/settings"
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

// Settings is email's contribution to the settings surface (docs/16): the
// initiator allowlist, the one DYNAMIC setting proving the reshape path. Its
// state stays in email.Model.Allowlist; this is a read plus a whole-list-replace
// write (set-allowlist) over it, not a relocation.
func Settings() []settings.Setting {
	return []settings.Setting{{
		Key:   "initiators",
		Short: "Initiator allowlist",
		Long:  "The email addresses allowed to start new tasks. Anyone listed here may open a task by email; everyone else is ignored. Add or remove rows, then Save.",
		Owner: "email",
		Read:  func(v runtime.View) settings.Value { return AllowlistOf(v) },
		Write: func(val settings.Value) runtime.Msg {
			return msg.NewSetAllowlist(msg.SetAllowlistPayload{Addresses: []string(val.(Allowlist))})
		},
	}}
}
