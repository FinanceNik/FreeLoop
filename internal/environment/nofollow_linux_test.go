//go:build linux

package environment

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// The walk an older kernel gets in place of openat2 refuses a link anywhere on the
// path, as openat2 with RESOLVE_NO_SYMLINKS does, and opens what has none.
func TestOpeningWithoutFollowingLinks(t *testing.T) {
	base, _ := filepath.EvalSymlinks(t.TempDir())
	secret := filepath.Join(base, "secret")
	os.MkdirAll(secret, 0o755)
	os.WriteFile(filepath.Join(secret, "id"), []byte("PRIVATE"), 0o600)
	os.MkdirAll(filepath.Join(base, "real"), 0o755)
	os.WriteFile(filepath.Join(base, "real", "id"), []byte("public"), 0o644)
	os.Symlink(secret, filepath.Join(base, "linked"))
	os.Symlink(filepath.Join(secret, "id"), filepath.Join(base, "real", "last"))
	for name, open := range map[string]func(string, int) (*os.File, error){"openat2": openNoFollow, "walk": openWalking} {
		for _, p := range []string{filepath.Join(base, "linked", "id"), filepath.Join(base, "real", "last")} {
			if f, err := open(p, os.O_RDONLY); err == nil {
				b, _ := io.ReadAll(f)
				f.Close()
				t.Errorf("%s opened %s through a link: %q", name, p, b)
			}
		}
		f, err := open(filepath.Join(base, "real", "id"), os.O_RDONLY)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b, _ := io.ReadAll(f)
		f.Close()
		if string(b) != "public" {
			t.Errorf("%s read %q", name, b)
		}
	}
}
