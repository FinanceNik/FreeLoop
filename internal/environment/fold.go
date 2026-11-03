package environment

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// FoldPath spells a path so that two names macOS treats as one file compare
// equal. APFS ignores case and Unicode normalization, and folds fully: long s
// (U+017F) is s, the Kelvin sign is K, sharp s is ss, a ligature is its letters,
// and an accent can be one code point or two. strings.ToLower knows none of
// that, so a rule on secrets/ missed secret\u017f/, which opens the same folder.
// On a case-sensitive volume this errs towards refusing.
func FoldPath(p string) string {
	// A Caser keeps state, so each call has its own.
	return norm.NFC.String(cases.Fold().String(norm.NFC.String(p)))
}
