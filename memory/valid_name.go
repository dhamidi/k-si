package memory

import "regexp"

// nameRE is the memory-name slug rule: it must start with a letter or digit and
// then hold only letters, digits, dots, dashes, and underscores. This bans the
// path metacharacters that would let a name escape its box ("/", "..") or a leading
// dot (a hidden file), and the whitespace/markdown metacharacters that would inject
// raw into the in/MEMORY.md index (feature-memory.md hardening).
var nameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidName reports whether name is a safe memory slug: non-empty, at most 128
// bytes, and matching ^[A-Za-z0-9][A-Za-z0-9._-]*$ — so it is also a safe file name
// (no "/", no "..", no leading dot) and a safe index-line token (no spaces, no
// newlines, no markdown metacharacters). It is the single gate every write path
// funnels a name through: the reducer (canonical), the owner's /memory form, and
// the out/memory/ harvest (feature-memory.md).
func ValidName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	return nameRE.MatchString(name)
}
