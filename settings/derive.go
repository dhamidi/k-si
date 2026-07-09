package settings

import (
	"flag"
	"fmt"
	"reflect"
	"strconv"
)

// FormOf is the DEFAULT former: it derives a value's form from its Go type by
// reflection, covering only the OBVIOUS flat kinds —
//
//	string / a flag.Value leaf → KindText
//	a bounded int              → KindNumber
//
// and NOTHING else. It never infers choice, secret, file, group, or list: there
// is no safe way to guess from a Go type that a string is a secret, so a type
// gets those kinds only by IMPLEMENTING ToForm and saying so (former.go). A flat
// leaf's ToForm is literally `return settings.FormOf(&v)`, delegating here; the
// web layer gets any value's form by calling its ToForm(), so FormOf never
// dispatches back to ToForm (which would recurse).
//
// The single field is named "value" — a leaf form has one control, scoped under
// the setting's key in the URL. Parse builds a fresh instance of the value's type
// and re-parses the raw string through its flag.Value.Set, the same contract the
// CLI flag uses, so render and parse can never diverge.
func FormOf(v Value) Form {
	et := reflect.TypeOf(v)
	if et.Kind() == reflect.Pointer {
		et = et.Elem()
	}
	ev := reflect.Indirect(reflect.ValueOf(v))

	var kind Kind
	var cur string
	switch ev.Kind() {
	case reflect.String:
		kind, cur = KindText, ev.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		kind, cur = KindNumber, strconv.FormatInt(ev.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		kind, cur = KindNumber, strconv.FormatUint(ev.Uint(), 10)
	default:
		// A type whose obvious kind is not flat (a slice, a struct) must implement
		// ToForm itself; FormOf renders it as a lone text field rather than
		// guessing a structure. This branch is unreachable for the shipped flat
		// settings — all leaves are string- or int-kinded.
		kind, cur = KindText, fmt.Sprint(ev.Interface())
	}

	f := Form{Fields: []Field{{Name: "value", Kind: kind, Value: cur}}}
	f.Parse = func(f Form) (Value, FieldErrors) {
		errs := FieldErrors{}
		raw := ""
		if len(f.Fields) > 0 {
			raw = f.Fields[0].Value
		}
		ptr := reflect.New(et) // *T, addressable so a pointer-receiver Set applies
		into, ok := ptr.Interface().(flag.Value)
		if !ok {
			errs.Set("value", fmt.Sprintf("%s is not settable from text", et))
			return nil, errs
		}
		errs.Parse("value", raw, into)
		if !errs.OK() {
			return nil, errs
		}
		val, ok := reflect.Indirect(ptr).Interface().(Value)
		if !ok {
			errs.Set("value", fmt.Sprintf("%s is not a setting value", et))
			return nil, errs
		}
		return val, nil
	}
	return f
}
