package mime

import (
	"net/mail"
	"strings"
)

// LocalPart extracts the local part (before the @) of an address, which is the
// route selector for inbound mail (docs/04): "pay@kasi.test" -> "pay". It
// tolerates display-name forms like "Pay Bot <pay@kasi.test>".
func LocalPart(address string) string {
	bare := address
	if a, err := mail.ParseAddress(address); err == nil {
		bare = a.Address
	} else {
		bare = strings.Trim(bare, " <>")
	}
	if i := strings.LastIndex(bare, "@"); i >= 0 {
		return bare[:i]
	}
	return bare
}

// CcList parses a Cc header value into bare addresses (display names dropped).
// Returns nil for an empty or unparseable header so callers can range freely.
func CcList(header string) []string {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}
	addrs, err := (&mail.AddressParser{}).ParseList(header)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.Address)
	}
	return out
}

// AppendReferences appends messageID to a References chain, returning a new
// slice (the input is never mutated). A blank id, or an id already at the tail,
// is a no-op so re-threading the same reply stays idempotent (docs/04).
func AppendReferences(references []string, messageID string) []string {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return references
	}
	if n := len(references); n > 0 && references[n-1] == messageID {
		return references
	}
	out := make([]string, len(references), len(references)+1)
	copy(out, references)
	return append(out, messageID)
}

// ReplySubject prefixes a subject with "Re: " for a reply, idempotently: a
// subject that already begins with a "Re:" (any casing) is returned unchanged
// so a long thread never accumulates "Re: Re: Re:".
func ReplySubject(subject string) string {
	trimmed := strings.TrimSpace(subject)
	if len(trimmed) >= 3 && strings.EqualFold(trimmed[:3], "re:") {
		return subject
	}
	return "Re: " + subject
}
