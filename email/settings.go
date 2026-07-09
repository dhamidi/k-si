package email

import (
	"fmt"
	"net/mail"

	"github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/settings"
)

// Allowlist is the initiator allowlist as a setting value — the DYNAMIC case: a
// list whose shape grows and shrinks as you edit it, so it implements ToForm
// explicitly (carrying an Update) rather than riding the default former. Each row
// is a non-sensitive address, which is what lets the reshape round-trip re-render
// freely — decision-004 keeps secret/file fields out of an Update body. The state
// stays in email.Model.Allowlist; this wraps it (docs/16).
type Allowlist []string

// AllowlistOf reads the initiator allowlist out of the model as the named value
// type (the -Of suffix leaves the bare name to the type).
func AllowlistOf(v runtime.View) Allowlist {
	return Allowlist(runtime.Slice[Model](v, "email").Allowlist)
}

// allowlistRow is the dotted-path prefix each address row carries ("addr.0",
// "addr.1", …) — the names the browser re-posts on every reshape round-trip.
const allowlistRow = "addr"

// addressValue is a flag.Value validating ONE email address — element-level
// parse-don't-validate for an allowlist row. An empty row is dropped, not an
// error, so a blank trailing field a user added and left is simply ignored.
type addressValue string

func (a *addressValue) Set(raw string) error {
	if raw == "" {
		*a = ""
		return nil
	}
	if _, err := mail.ParseAddress(raw); err != nil {
		return fmt.Errorf("must be a valid email address")
	}
	*a = addressValue(raw)
	return nil
}

func (a addressValue) String() string { return string(a) }

// ToForm builds the allowlist's dynamic form: one text field per address, an
// Update that adds or removes a row, and a Parse that validates each row through
// addressValue and REPLACES the whole list. The rows are non-sensitive, so the
// reshape may re-render their values without echoing anything secret.
func (a Allowlist) ToForm() settings.Form {
	f := settings.Form{Fields: rowFields(a)}

	f.Update = func(f settings.Form, ev settings.Event) settings.Form {
		switch ev.Op {
		case "add":
			f.Fields = append(f.Fields, settings.Field{Kind: settings.KindText, Label: "Address"})
		case "remove":
			if ev.Index >= 0 && ev.Index < len(f.Fields) {
				f.Fields = append(f.Fields[:ev.Index], f.Fields[ev.Index+1:]...)
			}
		}
		return renumberRows(f)
	}

	f.Parse = func(f settings.Form) (settings.Value, settings.FieldErrors) {
		out, errs := Allowlist{}, settings.FieldErrors{}
		for _, fld := range f.Fields {
			var addr addressValue
			if err := addr.Set(fld.Value); err != nil {
				errs.Set(fld.Name, err.Error())
			} else if addr != "" {
				out = append(out, string(addr))
			}
		}
		if !errs.OK() {
			return nil, errs
		}
		return out, errs
	}
	return f
}

func rowFields(a Allowlist) []settings.Field {
	fields := make([]settings.Field, len(a))
	for i, addr := range a {
		fields[i] = settings.Field{
			Name:  fmt.Sprintf("%s.%d", allowlistRow, i),
			Label: "Address",
			Kind:  settings.KindText,
			Value: addr,
		}
	}
	return fields
}

// renumberRows re-indexes row names to stay dense (addr.0, addr.1, …) after an
// add or remove, so the next round-trip's POST keys line up with the new shape.
func renumberRows(f settings.Form) settings.Form {
	for i := range f.Fields {
		f.Fields[i].Name = fmt.Sprintf("%s.%d", allowlistRow, i)
	}
	return f
}

// Settings is email's contribution: the initiator allowlist, the dynamic setting
// that proves the reshape path (docs/16). Its write is set-allowlist — the
// whole-value replace the incremental allow-sender cannot express.
func Settings() []settings.Setting {
	return []settings.Setting{{
		Key:   "initiators",
		Short: "Addresses allowed to start new tasks",
		Long:  "The initiator allowlist (docs/04). Anyone here may open a task by email; everyone else is ignored.",
		Owner: "email",
		Read:  func(v runtime.View) settings.Value { return AllowlistOf(v) },
		Write: func(val settings.Value) runtime.Msg {
			return msg.NewSetAllowlist(msg.SetAllowlistPayload{Addresses: []string(val.(Allowlist))})
		},
	}}
}
