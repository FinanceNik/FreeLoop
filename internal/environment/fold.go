package environment

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// FoldPath spells a path so that two names macOS treats as one file compare
// equal. APFS ignores case and Unicode normalization: long s (U+017F) is s, the
// Kelvin sign is K, and an accent can be one code point or two. strings.ToLower
// knows none of that, so a rule on secrets/ missed secret\u017f/, which opens the
// same folder. On a case-sensitive volume this errs towards refusing.
func FoldPath(p string) string {
	p = norm.NFC.String(p)
	var b strings.Builder
	b.Grow(len(p))
	for _, r := range p {
		least := r
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			if f < least {
				least = f
			}
		}
		b.WriteRune(least)
	}
	return b.String()
}
