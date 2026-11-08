package environment

import (
	"os/exec"
	"reflect"
	"slices"
	"testing"
)

func TestShellFor(t *testing.T) {
	have := func(paths ...string) func(string) bool {
		return func(p string) bool {
			for _, x := range paths {
				if x == p {
					return true
				}
			}
			return false
		}
	}
	tests := []struct {
		name          string
		goos          string
		passwd, shell string
		exists        func(string) bool
		login, bare   []string
	}{
		{"mac keeps zsh whatever the user has", "darwin", "/bin/bash", "/bin/bash", have("/bin/bash"), []string{"zsh", "-lc"}, []string{"zsh", "-fc"}},
		{"bash from the password database", "linux", "/bin/bash", "/usr/bin/fish", have("/bin/bash", "/usr/bin/fish"), []string{"/bin/bash", "-lc"}, []string{"/bin/bash", "--noprofile", "--norc", "-c"}},
		{"zsh from the password database", "linux", "/usr/bin/zsh", "", have("/usr/bin/zsh", "/bin/bash"), []string{"/usr/bin/zsh", "-lc"}, []string{"/usr/bin/zsh", "-fc"}},
		{"dash takes -c alone", "linux", "/bin/dash", "", have("/bin/dash"), []string{"/bin/dash", "-lc"}, []string{"/bin/dash", "-c"}},
		{"fish falls back to bash", "linux", "/usr/bin/fish", "/usr/bin/fish", have("/usr/bin/fish", "/bin/bash", "/bin/sh"), []string{"/bin/bash", "-lc"}, []string{"/bin/bash", "--noprofile", "--norc", "-c"}},
		{"$SHELL when the database has nothing", "linux", "", "/bin/ksh", have("/bin/ksh", "/bin/bash"), []string{"/bin/ksh", "-lc"}, []string{"/bin/ksh", "-c"}},
		{"a shell that is not installed is skipped", "linux", "/bin/zsh", "", have("/bin/bash"), []string{"/bin/bash", "-lc"}, []string{"/bin/bash", "--noprofile", "--norc", "-c"}},
		{"no bash falls back to sh", "linux", "/usr/bin/nu", "", have("/bin/sh"), []string{"/bin/sh", "-lc"}, []string{"/bin/sh", "-c"}},
		{"nothing found is sh on the PATH", "linux", "", "", have(), []string{"sh", "-lc"}, []string{"sh", "-c"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sh := ShellFor(tc.goos, tc.passwd, tc.shell, tc.exists)
			if !reflect.DeepEqual(sh.Login, tc.login) || !reflect.DeepEqual(sh.NoStartup, tc.bare) {
				t.Errorf("got login %v, no-startup %v; want %v, %v", sh.Login, sh.NoStartup, tc.login, tc.bare)
			}
		})
	}
}

func TestShellOfPasswd(t *testing.T) {
	passwd := "root:x:0:0:root:/root:/bin/bash\nbuild:x:1000:1000::/home/build:/usr/bin/zsh\n"
	if got := shellOfPasswd(passwd, "1000"); got != "/usr/bin/zsh" {
		t.Errorf("got %q", got)
	}
	if got := shellOfPasswd(passwd, "42"); got != "" {
		t.Errorf("got %q for a missing user", got)
	}
}

// Git Bash is found beside the git on PATH, then in the usual install folders, and
// WSL's bash.exe in System32 is never taken for it (OQ-043).
func TestGitBash(t *testing.T) {
	files := func(paths ...string) func(string) bool {
		return func(p string) bool { return slices.Contains(paths, p) }
	}
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	noGit := func(string) (string, error) { return "", exec.ErrNotFound }
	gitAt := func(p string) func(string) (string, error) { return func(string) (string, error) { return p, nil } }

	cases := []struct {
		name   string
		look   func(string) (string, error)
		env    map[string]string
		exists []string
		want   string
	}{
		{"beside git's cmd folder", gitAt(`D:\Tools\Git\cmd\git.exe`), nil, []string{`D:\Tools\Git\bin\bash.exe`}, `D:\Tools\Git\bin\bash.exe`},
		{"beside mingw64's git", gitAt(`C:\Git\mingw64\bin\git.exe`), nil, []string{`C:\Git\bin\bash.exe`}, `C:\Git\bin\bash.exe`},
		{"Program Files", noGit, map[string]string{"ProgramFiles": `C:\Program Files`}, []string{`C:\Program Files\Git\bin\bash.exe`}, `C:\Program Files\Git\bin\bash.exe`},
		{"a per-user install", noGit, map[string]string{"LocalAppData": `C:\Users\a\AppData\Local`}, []string{`C:\Users\a\AppData\Local\Programs\Git\bin\bash.exe`}, `C:\Users\a\AppData\Local\Programs\Git\bin\bash.exe`},
		{"never WSL's", gitAt(`C:\Windows\System32\git.exe`), nil, []string{`C:\Windows\System32\bin\bash.exe`}, ""},
		{"none", noGit, nil, nil, ""},
	}
	for _, c := range cases {
		if got := GitBash(c.look, env(c.env), files(c.exists...)); got != c.want {
			t.Errorf("%s: GitBash = %q, want %q", c.name, got, c.want)
		}
	}
}

// On Windows the shell is Git Bash, run as a login shell, and a machine without it
// still has a shell whose start explains what to install.
func TestShellForWindows(t *testing.T) {
	sh := ShellFor("windows", `C:\Program Files\Git\bin\bash.exe`, "", nil)
	if want := []string{`C:\Program Files\Git\bin\bash.exe`, "-lc"}; !slices.Equal(sh.Login, want) {
		t.Errorf("Login = %q, want %q", sh.Login, want)
	}
	if want := []string{`C:\Program Files\Git\bin\bash.exe`, "--noprofile", "--norc", "-c"}; !slices.Equal(sh.NoStartup, want) {
		t.Errorf("NoStartup = %q, want %q", sh.NoStartup, want)
	}
	if sh := ShellFor("windows", "", "", nil); sh.Path != "bash.exe" {
		t.Errorf("without Git Bash, Path = %q, want bash.exe", sh.Path)
	}
}

// A command on Windows keeps the variables Windows programs need, whatever the case
// of their names, and still not the app's own keys.
func TestInheritedOnWindows(t *testing.T) {
	got := inherited("windows", []string{"Path=C:\\Windows", "SystemRoot=C:\\Windows", "TEMP=C:\\t", "ANTHROPIC_API_KEY=sk", "=C:=C:\\"})
	want := []string{"Path=C:\\Windows", "SystemRoot=C:\\Windows", "TEMP=C:\\t"}
	if !slices.Equal(got, want) {
		t.Errorf("inherited = %q, want %q", got, want)
	}
	if got := inherited("darwin", []string{"SystemRoot=x", "PATH=/bin"}); !slices.Equal(got, []string{"PATH=/bin"}) {
		t.Errorf("on a Mac, inherited = %q", got)
	}
}
