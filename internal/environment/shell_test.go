package environment

import (
	"reflect"
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
