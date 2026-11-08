package environment

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// Shell is the shell this machine's commands run through (OQ-019). Commands
// are written for a POSIX shell, so it is always one that takes -c.
type Shell struct {
	// Path is the program: "zsh" on a Mac, the user's login shell or a stand-in
	// elsewhere.
	Path string
	// Login is the argv prefix that runs a command line in a login shell, so the
	// user's PATH and variables apply.
	Login []string
	// NoStartup is the prefix that reads no startup files, for the sandbox.
	NoStartup []string
}

// posixShells are the shells that take -l and -c and read commands the way the
// engine writes them. fish and nushell are left out on purpose.
var posixShells = []string{"bash", "zsh", "dash", "sh", "ksh"}

// ShellFor chooses a shell for goos from what the machine says: the user's
// login shell from the password database and $SHELL, in that order. exists
// reports whether a path can run. macOS keeps zsh, as it always has; elsewhere
// the first POSIX shell among the candidates, then bash, then sh.
func ShellFor(goos, passwd, envShell string, exists func(string) bool) Shell {
	if goos == "darwin" {
		return newShell("zsh")
	}
	for _, c := range []string{passwd, envShell} {
		if c != "" && isPOSIXShell(c) && exists(c) {
			return newShell(c)
		}
	}
	for _, c := range []string{"/bin/bash", "/usr/bin/bash", "/bin/sh", "/usr/bin/sh"} {
		if exists(c) {
			return newShell(c)
		}
	}
	return newShell("sh")
}

func isPOSIXShell(path string) bool {
	return filepath.IsAbs(path) && contains(posixShells, filepath.Base(path))
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func newShell(path string) Shell {
	sh := Shell{Path: path, Login: []string{path, "-lc"}}
	switch filepath.Base(path) {
	case "zsh":
		sh.NoStartup = []string{path, "-fc"}
	case "bash":
		sh.NoStartup = []string{path, "--noprofile", "--norc", "-c"}
	default:
		// dash, sh and ksh read no startup file without -l (ksh reads $ENV only
		// when interactive).
		sh.NoStartup = []string{path, "-c"}
	}
	return sh
}

var (
	shellOnce sync.Once
	shell     Shell
)

// LoginShell is the shell for this machine, chosen once.
func LoginShell() Shell {
	shellOnce.Do(func() {
		passwd := ""
		if runtime.GOOS != "darwin" {
			passwd = passwdShell()
		}
		shell = ShellFor(runtime.GOOS, passwd, os.Getenv("SHELL"), func(p string) bool {
			fi, err := os.Stat(p)
			return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
		})
	})
	return shell
}

// passwdShell is the login shell the password database has for the current
// user: /etc/passwd, then getent for accounts that live elsewhere (LDAP).
func passwdShell() string {
	uid := strconv.Itoa(os.Getuid())
	if b, err := os.ReadFile("/etc/passwd"); err == nil {
		if s := shellOfPasswd(string(b), uid); s != "" {
			return s
		}
	}
	if out, err := exec.Command("getent", "passwd", uid).Output(); err == nil {
		return shellOfPasswd(string(out), uid)
	}
	return ""
}

// shellOfPasswd finds the shell field of the line for uid.
func shellOfPasswd(passwd, uid string) string {
	for _, line := range strings.Split(passwd, "\n") {
		f := strings.Split(line, ":")
		if len(f) >= 7 && f[2] == uid {
			return strings.TrimSpace(f[6])
		}
	}
	return ""
}

// InstallHint is how to say what to install for a program: Homebrew's command
// on a Mac, and a pointer to the system's package manager elsewhere, where
// "brew install" is not what the user has.
func InstallHint(formula string) string {
	if runtime.GOOS == "darwin" {
		return "brew install " + formula
	}
	return "with your package manager"
}
