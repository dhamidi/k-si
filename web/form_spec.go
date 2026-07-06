package web

import (
	"encoding/json"
	"fmt"
)

// FieldSpec is one field of the agent-authored form spec (decision-005): the
// spec-driven type shared by render (view_request) and bind (AnswerRequestForm).
// The agent describes WHAT it needs; the page and the submit are generated from
// that description, so there is no bespoke page or struct per request type.
type FieldSpec struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
}

// The field types an agent may request (docs/08 §"Agent request forms").
const (
	FieldText     = "text"     // a single-line <input type=text>
	FieldLongText = "longtext" // a multi-line <textarea>
	FieldChoice   = "choice"   // a <select> over Options
	FieldFile     = "file"     // an <input type=file>, stored in archive
	FieldSecret   = "secret"   // a masked <input type=password>, written to secrets
)

// ParseFormSpec decodes UIRequest.FormSpec — the agent-authored JSON array of
// field descriptors — into typed FieldSpecs, validating that every field names
// a known type and carries a name and label. An empty or malformed spec is an
// error, not a silent empty form: the request is unanswerable without it.
func ParseFormSpec(b []byte) ([]FieldSpec, error) {
	var fields []FieldSpec
	if err := json.Unmarshal(b, &fields); err != nil {
		return nil, fmt.Errorf("form spec: %w", err)
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("form spec: no fields")
	}
	for i, f := range fields {
		if f.Name == "" {
			return nil, fmt.Errorf("form spec: field %d has no name", i)
		}
		if f.Label == "" {
			return nil, fmt.Errorf("form spec: field %q has no label", f.Name)
		}
		switch f.Type {
		case FieldText, FieldLongText, FieldChoice, FieldFile, FieldSecret:
			// ok
		default:
			return nil, fmt.Errorf("form spec: field %q has unknown type %q", f.Name, f.Type)
		}
	}
	return fields, nil
}
