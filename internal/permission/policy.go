package permission

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// Policy holds the fixed path rules: locations no harness may read in any
// mode (SEC-013) and locations no harness may write in any mode (SEC-005).
type Policy struct {
	neverRead  []string
	neverWrite []string
}

// NewPolicy builds the fixed rules for a user's home directory and the
// app's own data directory.
func NewPolicy(home, appDataDir string) *Policy {
	lib := filepath.Join(home, "Library")
	neverRead := []string{
		filepath.Join(home, ".ssh"),
		filepath.Join(home, ".gnupg"),
		filepath.Join(home, ".aws"),
		filepath.Join(lib, "Keychains"),
		filepath.Join(lib, "Cookies"),
		filepath.Join(lib, "Safari"),
		filepath.Join(lib, "Application Support", "Google", "Chrome"),
		filepath.Join(lib, "Application Support", "BraveSoftware"),
		filepath.Join(lib, "Application Support", "Microsoft Edge"),
		filepath.Join(lib, "Application Support", "Arc"),
		filepath.Join(lib, "Application Support", "Firefox"),
		appDataDir,
	}
	p := &Policy{}
	for _, d := range neverRead {
		p.neverRead = append(p.neverRead, canonical(d)...)
	}
	p.neverWrite = p.neverRead
	return p
}

// canonical returns the spellings of p that checks must cover.
func canonical(p string) []string { return environment.Spellings(p) }

// within reports whether p is dir or inside it. It ignores case: macOS
// volumes are case-insensitive by default, so ~/.SSH is ~/.ssh and .GIT is
// .git. On a case-sensitive volume this errs towards refusing.
func within(p, dir string) bool {
	p, dir = strings.ToLower(p), strings.ToLower(dir)
	return p == dir || strings.HasPrefix(p, dir+string(filepath.Separator))
}

func withinAny(p string, dirs []string) bool {
	for _, d := range dirs {
		if within(p, d) {
			return true
		}
	}
	return false
}

// readRefusal explains why p may never be read, or returns "".
func (p *Policy) readRefusal(path string) string {
	if withinAny(path, p.neverRead) {
		return fmt.Sprintf("%s is a protected location and is never readable", path)
	}
	return ""
}

// writeRefusal explains why path may never be written in a session rooted
// at root, or returns "".
func (p *Policy) writeRefusal(root, path string) string {
	roots := canonical(root)
	switch {
	case withinAny(path, p.neverWrite):
		return fmt.Sprintf("%s is a protected location and is never writable", path)
	case !withinAny(path, roots):
		return fmt.Sprintf("%s is outside the project root %s", path, root)
	}
	for _, r := range roots {
		if within(path, filepath.Join(r, ".git")) {
			return fmt.Sprintf("%s is inside .git", path)
		}
	}
	return ""
}

func (p *Policy) insideRoot(root, path string) bool {
	return withinAny(path, canonical(root))
}

// Guard returns the environment.Guard for a session rooted at root. It is the
// last line: the broker refuses the same paths before a tool runs, and the
// guard stops anything that reaches the file system regardless.
func (p *Policy) Guard(root string) environment.Guard {
	return guard{p: p, root: root}
}

type guard struct {
	p    *Policy
	root string
}

func (g guard) CheckRead(path string) error {
	if why := g.p.readRefusal(path); why != "" {
		return fmt.Errorf("%w: %s", environment.ErrProtected, why)
	}
	return nil
}

func (g guard) CheckWrite(path string) error {
	if why := g.p.writeRefusal(g.root, path); why != "" {
		return fmt.Errorf("%w: %s", environment.ErrProtected, why)
	}
	return nil
}
