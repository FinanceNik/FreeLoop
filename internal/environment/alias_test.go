package environment

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// On a Mac the data volume's own name is another spelling of every path on it, and it is
// a mount, not a link, so resolving symlinks leaves it as it is (SEC-005).
func TestTheDataVolumeSpellingIsDropped(t *testing.T) {
	const vol = "/System/Volumes/Data"
	for in, want := range map[string]string{
		vol + "/Users/me/.ssh/id_ed25519": "/Users/me/.ssh/id_ed25519",
		vol + "/Users/me":                 "/Users/me",
		vol + "/private/var/folders":      "/private/var/folders",
		vol:                               "/",
		vol + "/":                         "/",
		vol + "x/Users":                   vol + "x/Users", // another folder
		"/Users/me":                       "/Users/me",
		"/Volumes/Data/Users/me":          "/Volumes/Data/Users/me",
		"":                                "",
	} {
		if got := withoutDataVolume(in, vol); got != want {
			t.Errorf("withoutDataVolume(%q) = %q, want %q", in, got, want)
		}
	}
	// A system with no such volume leaves every path alone.
	if got := withoutDataVolume(vol+"/Users/me", ""); got != vol+"/Users/me" {
		t.Errorf("without a volume: %q", got)
	}
}

func TestAPathThroughTheDataVolumeResolvesToTheSamePlace(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the data volume is macOS's")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	l := &Local{root: Canonical(t.TempDir())}
	direct := l.Resolve(filepath.Join(home, ".ssh", "id_ed25519"))
	alias := l.Resolve("/System/Volumes/Data" + filepath.Join(home, ".ssh", "id_ed25519"))
	if alias != direct {
		t.Errorf("through the data volume %q, directly %q", alias, direct)
	}
	if got := Spellings("/System/Volumes/Data" + home); len(got) < 2 || got[len(got)-1] != Canonical(home) {
		t.Errorf("Spellings through the data volume: %q", got)
	}
}

// Windows takes `\\?\C:\Users\me` for `C:\Users\me`, and the UNC form for a share's path;
// neither is a link, so resolving leaves them as they are (SEC-005, NFR-XPLAT-003).
func TestTheVerbatimPrefixOnWindowsIsDropped(t *testing.T) {
	for in, want := range map[string]string{
		`\\?\C:\Users\me\.ssh\id_ed25519`: `C:\Users\me\.ssh\id_ed25519`,
		`//?/C:/Users/me/.ssh`:            `C:/Users/me/.ssh`,
		`\\.\C:\Users\me`:                 `C:\Users\me`,
		`\\?\UNC\server\share\x`:          `\\server\share\x`,
		`C:\Users\me`:                     `C:\Users\me`,
		`\\server\share\x`:                `\\server\share\x`,
		`\\?\Volume{1234}\x`:              `\\?\Volume{1234}\x`, // not a drive: left, and refused as outside
		``:                                ``,
	} {
		if got := withoutVerbatimPrefix(in, true); got != want {
			t.Errorf("withoutVerbatimPrefix(%q) = %q, want %q", in, got, want)
		}
		// Elsewhere a backslash is a name's own character.
		if got := withoutVerbatimPrefix(in, false); got != in {
			t.Errorf("on a system that is not Windows, %q became %q", in, got)
		}
	}
}
