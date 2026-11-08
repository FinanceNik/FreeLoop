package engine

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// What differs when the local environment is Windows (NFR-XPLAT-003). Commands
// there run through Git Bash (OQ-043), which spells paths its own way, /c/Users/a
// for C:\Users\a, while programs the app starts directly want Windows' spelling.
// Each helper is a pure function of the system's name, so both sides are tested on
// any machine; the callers pass goruntime.GOOS.

// localWindows reports an environment whose commands run on this Windows machine,
// as against in a container or on another machine, which are Linux whatever the
// host is.
func localWindows(env environment.Environment) bool {
	return goruntime.GOOS == "windows" && env.Kind() == environment.KindLocal
}

// neutralDirFor is neutralDir for env: on Windows "/" is not a path a program can
// start in, and would be read as the project's own folder, so it is the system
// drive's root there.
func neutralDirFor(env environment.Environment) string {
	if localWindows(env) {
		return neutralDirOn("windows", os.Getenv("SystemDrive"))
	}
	return neutralDir
}

// neutralDirOn is the folder outside any project to run a probe in on goos.
func neutralDirOn(goos, systemDrive string) string {
	if goos != "windows" {
		return neutralDir
	}
	if len(systemDrive) != 2 || systemDrive[1] != ':' {
		systemDrive = "C:"
	}
	return systemDrive + `\`
}

// bashPath spells a local path for a command line Git Bash runs on goos:
// C:\Users\a becomes /c/Users/a and \\host\share\a //host/share/a, since a colon
// in PATH separates entries and a backslash escapes. Elsewhere p is as it is.
func bashPath(goos, p string) string {
	if goos != "windows" {
		return p
	}
	p = strings.ReplaceAll(p, `\`, "/")
	if len(p) >= 2 && p[1] == ':' && isLetter(p[0]) {
		return "/" + strings.ToLower(p[:1]) + p[2:]
	}
	return p
}

// windowsPath is bashPath's reverse, for a path Git Bash printed: /c/Users/a is
// C:\Users\a. A path in Git's own tree, such as /usr/bin/x, has no drive and is
// left for the caller to refuse.
func windowsPath(p string) string {
	if len(p) >= 2 && p[0] == '/' && isLetter(p[1]) && (len(p) == 2 || p[2] == '/') {
		return strings.ToUpper(p[1:2]) + `:\` + strings.ReplaceAll(strings.TrimPrefix(p[2:], "/"), "/", `\`)
	}
	if strings.HasPrefix(p, "//") {
		return strings.ReplaceAll(p, "/", `\`)
	}
	return p
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// exeName is a program's file name on goos: rtk.exe on Windows.
func exeName(goos, program string) string {
	if goos == "windows" && !strings.EqualFold(filepath.Ext(program), ".exe") {
		return program + ".exe"
	}
	return program
}

// locateCommand is the command line that prints where program is on goos's
// login PATH: on Windows in Windows' spelling, through Git's cygpath, which also
// knows where Git Bash's own /usr/bin is.
func locateCommand(goos, program string) string {
	if goos == "windows" {
		return "p=$(command -v " + shellQuote(program) + `) && case "$p" in /*) cygpath -w "$p" ;; *) printf '%s\n' "$p" ;; esac`
	}
	return "command -v " + shellQuote(program)
}

// locatedPath reads where `command -v` found a program, the last line of out,
// since startup files can print before it: an absolute path, in Windows' spelling
// on Windows (Git Bash prints /c/...). "" for an alias, a function or a builtin,
// which nothing can run directly.
func locatedPath(goos, out string) string {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(out, "\r\n", "\n")), "\n")
	p := strings.TrimSpace(lines[len(lines)-1])
	if goos == "windows" {
		p = windowsPath(p)
		// Drive letter and a backslash, or a UNC path.
		if len(p) >= 3 && isLetter(p[0]) && p[1] == ':' && p[2] == '\\' || strings.HasPrefix(p, `\\`) {
			return p
		}
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		return ""
	}
	return p
}

// fileURIPath is a slash-separated absolute path as a file URI's path: C:/Users/a
// is /C:/Users/a, so the URI is file:///C:/Users/a, and //host/share/a stays as it
// is, a URI with a host. A Unix path is already one.
func fileURIPath(p string) string {
	if strings.HasPrefix(p, "/") {
		return p
	}
	return "/" + p
}
