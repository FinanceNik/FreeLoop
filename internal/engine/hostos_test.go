package engine

import (
	"archive/zip"
	"bytes"
	"slices"
	"strings"
	"testing"
)

// NFR-XPLAT-003: what differs on Windows, tested on any system through the system's
// name.

func TestGitBashPaths(t *testing.T) {
	for in, want := range map[string]string{
		`C:\Users\a\AppData\Local\freeloop\tools\rtk-1.1.0`: "/c/Users/a/AppData/Local/freeloop/tools/rtk-1.1.0",
		`D:\`:                "/d/",
		`\\host\share\a`:     "//host/share/a",
		"/already/unix":      "/already/unix",
		`relative\path\a.go`: "relative/path/a.go",
	} {
		if got := bashPath("windows", in); got != want {
			t.Errorf("bashPath(windows, %q) = %q, want %q", in, got, want)
		}
	}
	if got := bashPath("darwin", `/Users/a/b\c`); got != `/Users/a/b\c` {
		t.Errorf("bashPath(darwin) changed the path: %q", got)
	}
	for in, want := range map[string]string{
		"/c/Users/a/bin/rtk": `C:\Users\a\bin\rtk`,
		"/d":                 `D:\`,
		"//host/share/a":     `\\host\share\a`,
		"/usr/bin/rtk":       "/usr/bin/rtk",
	} {
		if got := windowsPath(in); got != want {
			t.Errorf("windowsPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLocatedProgram(t *testing.T) {
	for _, c := range []struct{ goos, out, want string }{
		{"darwin", "/opt/homebrew/bin/rtk\n", "/opt/homebrew/bin/rtk"},
		{"linux", "welcome from .profile\n/usr/local/bin/rtk\n", "/usr/local/bin/rtk"},
		{"linux", "rtk\n", ""}, // an alias or a function
		{"windows", "C:\\Users\\a\\.cargo\\bin\\rtk.exe\r\n", `C:\Users\a\.cargo\bin\rtk.exe`},
		{"windows", "/c/Users/a/bin/rtk\n", `C:\Users\a\bin\rtk`},
		{"windows", `\\host\tools\rtk.exe`, `\\host\tools\rtk.exe`},
		{"windows", "rtk\n", ""},
		{"windows", "/usr/bin/rtk\n", ""},
	} {
		if got := locatedPath(c.goos, c.out); got != c.want {
			t.Errorf("locatedPath(%s, %q) = %q, want %q", c.goos, c.out, got, c.want)
		}
	}
	if got := locateCommand("darwin", "rtk"); got != "command -v 'rtk'" {
		t.Errorf("locateCommand(darwin) = %q", got)
	}
	if got := locateCommand("windows", "rtk"); !strings.Contains(got, "cygpath -w") || !strings.Contains(got, "command -v 'rtk'") {
		t.Errorf("locateCommand(windows) = %q", got)
	}
}

func TestProgramNamesAndFolders(t *testing.T) {
	if exeName("windows", "rtk") != "rtk.exe" || exeName("windows", "rtk.EXE") != "rtk.EXE" || exeName("linux", "rtk") != "rtk" {
		t.Error("exeName")
	}
	if got := neutralDirOn("windows", "D:"); got != `D:\` {
		t.Errorf("neutralDirOn(windows, D:) = %q", got)
	}
	if got := neutralDirOn("windows", ""); got != `C:\` {
		t.Errorf("neutralDirOn(windows, \"\") = %q", got)
	}
	if got := neutralDirOn("darwin", "C:"); got != "/" {
		t.Errorf("neutralDirOn(darwin) = %q", got)
	}
	if got := fileURIPath("C:/Users/a/x.go"); got != "/C:/Users/a/x.go" {
		t.Errorf("fileURIPath = %q", got)
	}
	if got := fileURIPath("/Users/a/x.go"); got != "/Users/a/x.go" {
		t.Errorf("fileURIPath = %q", got)
	}
	for _, c := range []struct{ goos, home, local, want string }{
		{"darwin", "/Users/a", "", "/Users/a/Library/Caches/Freeloop"},
		{"linux", "/home/a", "", "/home/a/.cache/freeloop"},
		{"windows", "/home/a", "/somewhere/Local", "/somewhere/Local/freeloop"},
		{"windows", "/home/a", "", "/home/a/AppData/Local/freeloop"},
	} {
		if got := appCacheBase(c.goos, c.home, c.local); got != c.want {
			t.Errorf("appCacheBase(%s) = %q, want %q", c.goos, got, c.want)
		}
	}
}

// OQ-044: Windows has no native sandbox; the form says so, and so does a session.
func TestNoSandboxOnWindows(t *testing.T) {
	if why := sandboxRefusal("windows", false); !strings.Contains(why, "Windows has no native sandbox") || !strings.Contains(why, "Docker") {
		t.Errorf("sandboxRefusal(windows) = %q", why)
	}
	if why := sandboxRefusal("windows", true); why != "" {
		t.Errorf("a Windows confiner was refused: %q", why)
	}
	if why := sandboxRefusal("linux", false); why != "" {
		t.Errorf("sandboxRefusal(linux) = %q", why)
	}
	if !strings.Contains(noSandboxReason("linux"), "no sandbox") {
		t.Error(noSandboxReason("linux"))
	}
}

func TestGitOnWindows(t *testing.T) {
	win := safeGitOn("windows", `D:\Windows`)
	if !slices.Contains(win, `core.hooksPath=D:\Windows\System32\freeloop-no-hooks`) || slices.Contains(win, "core.hooksPath=/dev/null") {
		t.Errorf("safeGitOn(windows) = %q", win)
	}
	if got := safeGitOn("windows", ""); !slices.Contains(got, `core.hooksPath=C:\Windows\System32\freeloop-no-hooks`) {
		t.Errorf("safeGitOn(windows, \"\") = %q", got)
	}
	if got := safeGitOn("darwin", `C:\Windows`); !slices.Equal(got, safeGit) {
		t.Errorf("safeGitOn(darwin) = %q", got)
	}
	if _, ok := gitEnvOn("windows", nil)["GIT_CONFIG_NOSYSTEM"]; ok {
		t.Error("Windows' git does not read its system config")
	}
	if gitEnvOn("linux", map[string]string{"A": "b"})["GIT_CONFIG_NOSYSTEM"] != "1" || gitEnvOn("linux", map[string]string{"A": "b"})["A"] != "b" {
		t.Error("gitEnvOn(linux)")
	}
}

func TestInstallHints(t *testing.T) {
	if !strings.Contains(nodeHint("windows"), "winget") || !strings.Contains(nodeHint("darwin"), "brew") || strings.Contains(nodeHint("linux"), "brew") {
		t.Errorf("nodeHint: %q %q %q", nodeHint("windows"), nodeHint("darwin"), nodeHint("linux"))
	}
	if !strings.Contains(uvHint("windows"), "winget") || !strings.Contains(uvHint("darwin"), "brew") {
		t.Error("uvHint")
	}
}

func TestAnonymizeWindowsHome(t *testing.T) {
	in := `open C:\Users\Niklas\proj\a.go; git: C:/Users/Niklas/proj; bash: /c/Users/Niklas/proj; json: "C:\\Users\\Niklas\\x"; lower c:\users\niklas\y; other C:\Users\Nik`
	got := anonymizeHome("windows", `C:\Users\Niklas`, in)
	want := `open ~\proj\a.go; git: ~/proj; bash: ~/proj; json: "~\\x"; lower ~\y; other C:\Users\Nik`
	if got != want {
		t.Errorf("anonymizeHome =\n%s\nwant\n%s", got, want)
	}
	if got := anonymizeHome("darwin", "/Users/a/", "/Users/a/x"); got != "~/x" {
		t.Errorf("anonymizeHome(darwin) = %q", got)
	}
}

// Windows drops the dots and spaces a name ends with, so .freeloop./hooks.yaml is
// .freeloop\hooks.yaml there.
func TestActedOnFileWindowsSpellings(t *testing.T) {
	for _, name := range []string{".freeloop./hooks.yaml", ".gitmodules. ", `.vscode\settings.json`, ".github. /workflows/ci.yml"} {
		if !actedOnFile(name) {
			t.Errorf("%s is not acted on", name)
		}
	}
	if actedOnFile("src/main.go") {
		t.Error("src/main.go is acted on")
	}
}

// RTK's Windows build is a .zip holding rtk.exe.
func TestProgramFromZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{"LICENSE": "license", "rtk.exe": "MZ program"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	got, err := programFromArchive(buf.Bytes(), "rtk.exe")
	if err != nil || string(got) != "MZ program" {
		t.Fatalf("programFromArchive = %q, %v", got, err)
	}
	if _, err := programFromArchive(buf.Bytes(), "rtk"); err == nil {
		t.Error("a zip without the program was accepted")
	}
}
