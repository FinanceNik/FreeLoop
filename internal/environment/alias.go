package environment

import (
	"runtime"
	"strings"
)

// dataVolume is the name macOS gives the volume that holds /Users, /private and the other
// folders it shows at the top, which are firmlinks into it. No other system has one.
var dataVolume = dataVolumeOn(runtime.GOOS)

func dataVolumeOn(goos string) string {
	if goos == "darwin" {
		return "/System/Volumes/Data"
	}
	return ""
}

// withoutDataVolume spells p without the data volume's own name, vol: on a Mac
// /System/Volumes/Data/Users/me is /Users/me. The name is a mount and not a link, so
// resolving symlinks leaves it alone, and a rule on /Users/me/.ssh compared as strings
// missed it (SEC-005). Another folder that only starts with the same letters is left alone.
func withoutDataVolume(p, vol string) string {
	if vol == "" || !strings.HasPrefix(p, vol) {
		return p
	}
	rest := p[len(vol):]
	switch {
	case rest == "":
		return "/"
	case rest[0] == '/':
		return rest
	}
	return p
}

// withoutVerbatimPrefix spells a Windows path without its `\\?\` or `\\.\` prefix, which
// names the same place: `\\?\C:\Users\me` is `C:\Users\me` and `\\?\UNC\s\x` is `\\s\x`.
// Either slash may be used. Other systems read a backslash as part of a name, so the path
// is left as it is there, and so is a prefix that is not followed by a drive or UNC.
func withoutVerbatimPrefix(p string, windows bool) string {
	if !windows || len(p) < 4 {
		return p
	}
	sep := func(c byte) bool { return c == '\\' || c == '/' }
	if !(sep(p[0]) && sep(p[1]) && (p[2] == '?' || p[2] == '.') && sep(p[3])) {
		return p
	}
	rest := p[4:]
	if len(rest) >= 2 && rest[1] == ':' && (rest[0]|0x20) >= 'a' && (rest[0]|0x20) <= 'z' {
		return rest
	}
	if len(rest) >= 4 && strings.EqualFold(rest[:3], "UNC") && sep(rest[3]) {
		return string(p[0]) + string(p[1]) + rest[4:]
	}
	return p
}

// Unaliased spells p as one place has one name: without macOS's data volume name, and
// without Windows' verbatim prefix. A check on a path calls it first, so the spelling a
// caller chose does not decide the answer. Other systems' paths come back as they are.
func Unaliased(p string) string {
	return withoutVerbatimPrefix(withoutDataVolume(p, dataVolume), runtime.GOOS == "windows")
}
