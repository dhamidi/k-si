package web

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AnswerRequestForm — the spec-driven form object for a UI-request answer
// (decision-005). Unlike a static flag.Value form, its fields are known only at
// runtime, so it is bound dynamically from the request's parsed spec. It does
// NOT construct the answer message itself: files and secrets need web-edge I/O
// (archive writes, secret writes) that only the handler can perform (decision-004).
// It exposes the parsed, validated inputs — text values, uploaded files, and
// secret plaintexts — plus Errors for an invalid re-render.
type AnswerRequestForm struct {
	spec []FieldSpec

	// Values holds the submitted text/longtext/choice inputs by field name.
	Values map[string]string
	// Files holds the uploaded file for each file field, by field name.
	Files map[string]UploadedFile
	// Secrets holds secret plaintexts by field name. These go ONLY to the
	// secrets writer at the edge — never into Values, the model, the view, or a
	// log line (decision-004).
	Secrets map[string]string
	// Errors carries per-field validation messages for re-render.
	Errors FormErrors
}

// UploadedFile is one file field's submitted content, read at bind time so the
// handler can hand its bytes to the archive without reopening the request.
type UploadedFile struct {
	Filename    string
	ContentType string
	Bytes       []byte
}

// BindAnswerRequestForm reads the submission against the spec: FormValue for
// text/longtext/choice/secret, FormFile for file. Binding never fails as an
// HTTP error — a file that will not read becomes a field error in Validate.
func BindAnswerRequestForm(r *http.Request, spec []FieldSpec) AnswerRequestForm {
	f := AnswerRequestForm{
		spec:    spec,
		Values:  map[string]string{},
		Files:   map[string]UploadedFile{},
		Secrets: map[string]string{},
		Errors:  FormErrors{},
	}

	for _, fs := range spec {
		switch fs.Type {
		case FieldFile:
			file, header, err := r.FormFile(fs.Name)
			if err != nil {
				// http.ErrMissingFile (or no multipart part): treat as absent;
				// Validate reports it only if the field is required.
				continue
			}
			bytes, readErr := io.ReadAll(file)
			file.Close()
			if readErr != nil {
				f.Errors.Set(fs.Name, "could not read the uploaded file")
				continue
			}
			f.Files[fs.Name] = UploadedFile{
				Filename:    header.Filename,
				ContentType: header.Header.Get("Content-Type"),
				Bytes:       bytes,
			}
		case FieldSecret:
			// Kept apart from Values so a plaintext can never leak into a re-render.
			f.Secrets[fs.Name] = r.FormValue(fs.Name)
		default: // text, longtext, choice
			f.Values[fs.Name] = strings.TrimSpace(r.FormValue(fs.Name))
		}
	}
	return f
}

// Validate records a field error for each missing required field.
func (f AnswerRequestForm) Validate() AnswerRequestForm {
	for _, fs := range f.spec {
		if !fs.Required {
			continue
		}
		switch fs.Type {
		case FieldFile:
			if _, ok := f.Files[fs.Name]; !ok {
				f.Errors.Set(fs.Name, requiredMsg(fs.Label))
			}
		case FieldSecret:
			if f.Secrets[fs.Name] == "" {
				f.Errors.Set(fs.Name, requiredMsg(fs.Label))
			}
		default:
			if f.Values[fs.Name] == "" {
				f.Errors.Set(fs.Name, requiredMsg(fs.Label))
			}
		}
	}
	return f
}

// Valid reports whether the answer may be turned into a message.
func (f AnswerRequestForm) Valid() bool { return len(f.Errors) == 0 }

func requiredMsg(label string) string {
	return fmt.Sprintf("%s is required", label)
}
