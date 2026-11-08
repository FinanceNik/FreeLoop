//go:build linux

package linux

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// bwrapPaths are where bubblewrap is looked for: absolute, since a PATH the agent
// can influence must not choose what enforces the sandbox.
var bwrapPaths = []string{"/usr/bin/bwrap", "/bin/bwrap", "/usr/local/bin/bwrap"}

// maxPatternWalk bounds how many entries a deny rule's wildcard looks through when a
// command starts. A project bigger than that fails closed: its commands do not run
// while such a rule is in place.
const maxPatternWalk = 200_000

// Bubblewrap confines commands with bubblewrap on Linux (ENV-009, OQ-037), as Seatbelt
// does on macOS. Seatbelt allows by default and denies; bubblewrap builds what the
// command sees out of mounts, so the guarantees are kept another way:
//
//   - other programs: a private process namespace, so none can be seen, signalled or
//     read, and a new terminal session, so none can be fed input;
//   - files: the root read-only; the project, its caches and temp files writable;
//     the home folder emptied, with what may be read mounted back;
//   - sockets: /tmp, /run and the home folder are fresh, so Docker's, the SSH agent's,
//     D-Bus's and the keyring's sockets are out of reach;
//   - network: none, or the sandbox's own loopback, in a namespace of its own; or the
//     host's when the environment allows all of it.
//
// What it cannot do is protect a path that does not exist when the command starts:
// only existing paths can be mounted read-only.
type Bubblewrap struct{}

// Confine wraps argv in bwrap for c. An error means the command must not run.
func (Bubblewrap) Confine(c environment.Confinement, argv []string) ([]string, error) {
	bwrap := ""
	for _, p := range bwrapPaths {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			bwrap = p
			break
		}
	}
	if bwrap == "" {
		return nil, errors.New("the sandbox needs bubblewrap: install the bubblewrap package")
	}
	args, err := BubblewrapArgs(c)
	if err != nil {
		return nil, err
	}
	return append(append(append([]string{bwrap}, args...), "--"), argv...), nil
}

// BubblewrapArgs are bwrap's options for c, in order: later mounts go on top.
func BubblewrapArgs(c environment.Confinement) ([]string, error) {
	if c.Home == "" || len(c.Write) == 0 {
		return nil, errors.New("a sandbox needs a home folder and a project")
	}
	a := []string{
		"--die-with-parent", "--new-session",
		"--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-cgroup-try",
	}
	if c.Network != environment.NetworkAll {
		a = append(a, "--unshare-net")
	}
	a = append(a, "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "--tmpfs", "/tmp", "--tmpfs", "/run")
	// /var/run is usually a link to /run; a folder of its own gets emptied too.
	if fi, err := os.Lstat("/var/run"); err == nil && fi.IsDir() {
		a = append(a, "--tmpfs", "/var/run")
	}
	a = append(a, "--tmpfs", c.Home)

	// What can be read and written, outer folders first so inner mounts go on top.
	type mount struct{ path, kind string }
	var mounts []mount
	for _, p := range c.Read {
		mounts = append(mounts, mount{p, "--ro-bind"})
	}
	write := "--bind"
	if c.ReadOnly {
		write = "--ro-bind"
	}
	for _, p := range c.Write {
		mounts = append(mounts, mount{p, write})
	}
	for _, p := range c.Scratch {
		mounts = append(mounts, mount{p, "--bind"})
	}
	slices.SortStableFunc(mounts, func(x, y mount) int { return depth(x.path) - depth(y.path) })
	for _, m := range mounts {
		if exists(m.path) {
			a = append(a, m.kind, m.path, m.path)
		}
	}

	walked, err := matching(c, append(append(append([]string{}, c.Write...), c.Scratch...), c.Read...))
	if err != nil {
		return nil, err
	}
	// A folder mounted on itself cannot be renamed or removed (EBUSY), and what is
	// inside stays writable: .git cannot be moved aside, edited and moved back.
	for _, p := range append(slices.Clone(c.NoMove), walked.noMove...) {
		if isDir(p) {
			a = append(a, "--bind", p, p)
		}
	}
	// Never written: read-only on top of the project.
	for _, p := range append(slices.Clone(c.Protected), walked.protected...) {
		if exists(p) {
			a = append(a, "--ro-bind", p, p)
		}
	}
	// Neither read nor written: an empty folder or file in its place. Under the home
	// folder only what was mounted back is there to hide.
	var readOnly []string
	mounted := func(p string) bool {
		return slices.ContainsFunc(mounts, func(m mount) bool { return within(p, m.path) })
	}
	for _, p := range append(slices.Clone(c.NoAccess), walked.noAccess...) {
		if within(p, c.Home) && !mounted(p) {
			continue
		}
		switch {
		case isDir(p):
			a = append(a, "--tmpfs", p)
			readOnly = append(readOnly, p)
		case exists(p):
			a = append(a, "--ro-bind", "/dev/null", p)
		}
	}
	for _, p := range c.ReadAlways {
		if exists(p) {
			a = append(a, "--ro-bind", p, p)
		}
	}
	// Last, so the folders the mounts above needed could be made in them: nothing is
	// written to the emptied home folder or a hidden one, which would seem to work and
	// then be gone, and would let a command create the probe the self-test looks for.
	for _, p := range append(readOnly, c.Home) {
		a = append(a, "--remount-ro", p)
	}
	return a, nil
}

// within reports whether p is dir or inside it.
func within(p, dir string) bool {
	p, dir = filepath.Clean(p), filepath.Clean(dir)
	return p == dir || strings.HasPrefix(p, dir+string(filepath.Separator))
}

// walkedPaths are the existing paths a deny rule's wildcard matches.
type walkedPaths struct{ protected, noMove, noAccess []string }

// matching finds what the patterns of c match under roots, when there are any.
func matching(c environment.Confinement, roots []string) (walkedPaths, error) {
	var w walkedPaths
	compile := func(res []string) ([]*regexp.Regexp, error) {
		var out []*regexp.Regexp
		for _, re := range res {
			r, err := regexp.Compile(re)
			if err != nil {
				return nil, fmt.Errorf("a path pattern does not compile: %w", err)
			}
			out = append(out, r)
		}
		return out, nil
	}
	protected, err := compile(c.ProtectedPatterns)
	if err != nil {
		return w, err
	}
	noMove, err := compile(c.NoMovePatterns)
	if err != nil {
		return w, err
	}
	noAccess, err := compile(c.NoAccessPatterns)
	if err != nil {
		return w, err
	}
	if len(protected)+len(noMove)+len(noAccess) == 0 {
		return w, nil
	}
	any := func(rs []*regexp.Regexp, p string) bool {
		return slices.ContainsFunc(rs, func(r *regexp.Regexp) bool { return r.MatchString(p) })
	}
	seen, n := map[string]bool{}, 0
	for _, root := range roots {
		if !exists(root) {
			continue
		}
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if n++; n > maxPatternWalk {
				return fmt.Errorf("a deny rule's wildcard would look through more than %d files in %s; its commands do not run while the rule is in place", maxPatternWalk, root)
			}
			if seen[p] {
				return nil
			}
			seen[p] = true
			if any(noAccess, p) {
				w.noAccess = append(w.noAccess, p)
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if any(protected, p) {
				w.protected = append(w.protected, p)
			}
			if d.IsDir() && any(noMove, p) {
				w.noMove = append(w.noMove, p)
			}
			return nil
		})
		if err != nil {
			return w, err
		}
	}
	return w, nil
}

func depth(p string) int { return strings.Count(filepath.Clean(p), string(filepath.Separator)) }

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func isDir(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.IsDir()
}
