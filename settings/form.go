package settings

// Form is a runtime VALUE — a tree of typed fields carried and manipulated like
// any other value, never a static template (docs/16). Two closures make it live:
// Update changes its shape in response to an input event; Parse turns the filled
// form back into a typed value or structured errors, the inverse of ToForm.
type Form struct {
	Fields []Field

	// Update folds a shape-changing event into the form and returns the new form:
	// grow a list, drop a list item, reveal a field another field's value made
	// relevant. Pure: (form, event) → form. A fixed-shape form leaves this nil.
	Update func(f Form, ev Event) Form

	// Parse reads the submitted values off the form and produces the typed Value,
	// or the per-field errors that stop the write. This is ToForm's inverse and
	// the ONLY gate — there is no separate Validate pass to diverge from it. It
	// sees only the non-sensitive fields; secret/file fields are handled by the
	// web edge before Parse runs (decision-004). A render-only form (a Flow C spec
	// turned into a Form) leaves this nil.
	Parse func(f Form) (Value, FieldErrors)
}

// Field is one control: a name, a label, a control kind chosen from the Go type,
// its current raw string (leaf controls), options (a choice), nested fields (a
// group or list item), and any parse error. It generalises web.FieldView
// (decision-005) so a settings form and a Flow C request form render through the
// same control.
type Field struct {
	Name    string   // dotted path within the form: "addr.2", "smtp.host"
	Label   string   // human label shown beside the control
	Kind    Kind     // which control renders
	Value   string   // the raw current/submitted string (leaf kinds)
	Options []string // KindChoice
	Fields  []Field  // KindGroup members, KindList items
	Error   string   // this field's parse error, empty when it parsed
}

// Kind is the control a field renders as — the generalisation of the Flow C
// field types, adding number/list/group.
type Kind string

const (
	KindText     Kind = "text"     // <input type=text>
	KindLongText Kind = "longtext" // <textarea>
	KindChoice   Kind = "choice"   // <select> over Options
	KindSecret   Kind = "secret"   // masked <input type=password> (decision-004)
	KindFile     Kind = "file"     // <input type=file>, stored in archive
	KindNumber   Kind = "number"   // <input type=number>, a bounded int
	KindBool     Kind = "bool"     // <input type=checkbox>, an on/off toggle
	KindList     Kind = "list"     // repeated Fields, add/remove controls
	KindGroup    Kind = "group"    // a nested struct's fields
)

// True and False are the raw string values a KindBool field's Value carries — an
// unchecked checkbox submits nothing, so absence reads as False. Parsers compare
// a bool field's Value against True.
const (
	True  = "true"
	False = ""
)

// Event is the shape-changing input Update folds in: which list to grow or
// shrink, which dependent field to reveal.
type Event struct {
	Op    string // "add" | "remove"
	Field string // the target field's dotted path
	Index int    // for "remove": which list item
}

// FieldErrors maps a field's dotted path to its parse error — the structured,
// nested successor to web.FormErrors. Empty means the parse produced a value.
type FieldErrors map[string]string

// Set records the first error for a field; later errors for the same field keep
// the first, so parsing reads top-to-bottom and reports the primary problem
// (mirrors web.FormErrors.Set).
func (e FieldErrors) Set(field, message string) {
	if _, taken := e[field]; !taken {
		e[field] = message
	}
}

// Bind copies submitted raw strings onto the form's leaf fields by their dotted
// Name (recursing into groups and list items), returning the bound form — the
// pure "re-seed the shape with what the user typed" step every round-trip needs,
// so no server session is required (docs/16). A name absent from values leaves
// that field's value untouched.
func (f Form) Bind(values map[string]string) Form {
	f.Fields = bindFields(f.Fields, values)
	return f
}

func bindFields(fields []Field, values map[string]string) []Field {
	out := make([]Field, len(fields))
	for i, fld := range fields {
		if v, ok := values[fld.Name]; ok {
			fld.Value = v
		}
		if len(fld.Fields) > 0 {
			fld.Fields = bindFields(fld.Fields, values)
		}
		out[i] = fld
	}
	return out
}
