package environment

import (
	"errors"
	"fmt"
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
	// Path is the program: "zsh" on a Mac, Git Bash on Windows (OQ-038), the user's
	// login shell or a stand-in elsewhere.
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
// reports whether a path can run. macOS keeps zsh, as it always has; Windows has
// Git Bash, passed as passwd (GitBash finds it); elsewhere the first POSIX shell
// among the candidates, then bash, then sh.
func ShellFor(goos, passwd, envShell string, exists func(string) bool) Shell {
	if goos == "darwin" {
		return newShell("zsh")
	}
	if goos == "windows" {
		if passwd == "" {
			// Nothing to run commands with: starting one says what to install
			// (startHint).
			return newShell("bash.exe")
		}
		return newShell(passwd)
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

// GitBash finds Git for Windows' bash.exe, the shell commands run through on
// Windows (OQ-038): beside the git on PATH, then in the usual install folders.
// System32's bash.exe is WSL's launcher, not Git Bash, and is never chosen.
// lookPath is exec.LookPath, getenv os.Getenv and exists reports a file; the
// path is Windows', joined with backslashes whatever system runs the test.
func GitBash(lookPath func(string) (string, error), getenv func(string) string, exists func(string) bool) string {
	var candidates []string
	if git, err := lookPath("git.exe"); err == nil {
		// <root>\cmd\git.exe, <root>\bin\git.exe or <root>\mingw64\bin\git.exe.
		dir := winDir(git)
		for range 3 {
			candidates = append(candidates, dir+`\bin\bash.exe`)
			dir = winDir(dir)
		}
	}
	for _, v := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)", "LocalAppData"} {
		base := getenv(v)
		if base == "" {
			continue
		}
		if v == "LocalAppData" {
			base += `\Programs`
		}
		candidates = append(candidates, base+`\Git\bin\bash.exe`)
	}
	for _, c := range candidates {
		if strings.Contains(strings.ToLower(c), `\system32\`) {
			continue
		}
		if exists(c) {
			return c
		}
	}
	return ""
}

// winDir is the folder of a Windows path.
func winDir(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i > 0 {
		return p[:i]
	}
	return p
}

// startHint explains a program that would not start for want of the shell: on
// Windows, that Git for Windows has to be installed (OQ-038).
func startHint(err error) error {
	if runtime.GOOS == "windows" && errors.Is(err, exec.ErrNotFound) && LoginShell().Path == "bash.exe" {
		return fmt.Errorf("%w: Freeloop runs commands through Git Bash; install Git for Windows from https://git-scm.com/download/win and restart Freeloop", err)
	}
	return err
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
	switch strings.TrimSuffix(strings.ToLower(winBase(path)), ".exe") {
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

// winBase is the last part of a path with either separator.
func winBase(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		return p[i+1:]
	}
	return p
}

var (
	shellOnce sync.Once
	shell     Shell
)

// LoginShell is the shell for this machine, chosen once.
func LoginShell() Shell {
	shellOnce.Do(func() {
		passwd := ""
		switch runtime.GOOS {
		case "darwin":
		case "windows":
			passwd = GitBash(exec.LookPath, os.Getenv, func(p string) bool {
				fi, err := os.Stat(p)
				return err == nil && !fi.IsDir()
			})
		default:
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
	switch runtime.GOOS {
	case "darwin":
		return "brew install " + formula
	case "windows":
		return "with winget or from its website"
	}
	return "with your package manager"
}
