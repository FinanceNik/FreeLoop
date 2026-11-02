//go:build darwin

package darwin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// sandboxExec is an absolute path: a PATH the agent can influence must not
// choose what enforces the sandbox.
const sandboxExec = "/usr/bin/sandbox-exec"

// Seatbelt confines commands with macOS's sandbox (ENV-009). Apple marks
// sandbox-exec deprecated and offers nothing else for this; if it ever stops
// working, Confine or the environment's self-test fails and commands do not run.
type Seatbelt struct{}

// Confine wraps argv in sandbox-exec with a profile for c.
func (Seatbelt) Confine(c environment.Confinement, argv []string) ([]string, error) {
	if _, err := os.Stat(sandboxExec); err != nil {
		return nil, fmt.Errorf("%s is not on this Mac", sandboxExec)
	}
	profile, err := SeatbeltProfile(c)
	if err != nil {
		return nil, err
	}
	return append([]string{sandboxExec, "-p", profile}, argv...), nil
}

// SeatbeltProfile writes the profile. Later rules win over earlier ones.
//
// It allows by default and denies what matters, because a deny-by-default
// profile breaks ordinary tools in ways that are hard to foresee. File and
// network rules alone are not a sandbox, though: `open` asks a system service to
// start an app, which then runs outside. So every Mach service is denied, which
// also closes AppleScript, the clipboard, launchctl and the Keychain.
func SeatbeltProfile(c environment.Confinement) (string, error) {
	if c.Home == "" || len(c.Write) == 0 {
		return "", errors.New("a sandbox needs a home folder and a project")
	}
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	paths := func(kind string, ps []string) string {
		var out []string
		for _, p := range ps {
			out = append(out, "("+kind+" "+quote(p)+")")
		}
		return strings.Join(out, " ")
	}
	w("(version 1)")
	w("(allow default)")
	w("(deny appleevent-send)")
	w("(deny mach-lookup)")
	w("(deny mach-register)")
	// Not other programs: no killing them, and no reading their arguments and
	// environment, where keys live.
	w("(deny signal)")
	w("(allow signal (target same-sandbox))")
	w("(deny process-info*)")
	w("(allow process-info* (target same-sandbox))")
	w("(deny sysctl-read (sysctl-name-prefix \"kern.procargs\"))")

	if c.Certificates {
		w("(allow mach-lookup (global-name \"com.apple.trustd.agent\") (global-name \"com.apple.trustd\"))")
	}
	switch c.Network {
	case environment.NetworkAll:
		// Name lookups and certificate checks are services too.
		w("(allow mach-lookup (global-name \"com.apple.dnssd.service\") (global-name \"com.apple.trustd.agent\") (global-name \"com.apple.trustd\") (global-name \"com.apple.SystemConfiguration.configd\") (global-name \"com.apple.SystemConfiguration.DNSConfiguration\"))")
	case environment.NetworkNone:
		w("(deny network*)")
	default:
		w("(deny network*)")
		w("(allow network-bind (local ip \"localhost:*\"))")
		w("(allow network-inbound (local ip \"localhost:*\"))")
		w("(allow network-outbound (remote ip \"localhost:*\"))")
	}
	// A program's own Unix sockets, in its own folders, whatever else it may
	// reach. A socket is a way into a program running outside the sandbox: the
	// app's own API, Docker, the SSH agent, a tmux server that runs what it is
	// told. /tmp is left out for that reason: it is where other programs keep
	// theirs, and commands have their own TMPDIR.
	w("(deny network* (local unix-socket) (remote unix-socket))")
	for _, d := range append(append([]string{}, c.Write...), c.Scratch...) {
		if d == "/tmp" || d == "/private/tmp" {
			continue
		}
		w("(allow network-bind network-inbound network-outbound (local unix-socket (subpath %s)) (remote unix-socket (subpath %s)))", quote(d), quote(d))
	}
	// Name lookups go through this one.
	if c.Network == environment.NetworkAll {
		w("(allow network-outbound (remote unix-socket (literal \"/private/var/run/mDNSResponder\")))")
	}

	w("(deny file-write*)")
	w("(allow file-write* (literal \"/dev/null\") (literal \"/dev/zero\") (literal \"/dev/random\") (literal \"/dev/urandom\") (literal \"/dev/ptmx\") (literal \"/dev/dtracehelper\") (regex #\"^/dev/tty\") (regex #\"^/dev/fd/\"))")
	w("(allow file-read* (literal \"/dev/ptmx\"))")
	if len(c.Scratch) > 0 {
		w("(allow file-write* %s)", paths("subpath", c.Scratch))
	}
	if !c.ReadOnly {
		w("(allow file-write* %s)", paths("subpath", c.Write))
	}
	if len(c.Protected) > 0 {
		w("(deny file-write* %s %s)", paths("literal", c.Protected), paths("subpath", c.Protected))
	}
	for _, re := range c.ProtectedPatterns {
		if strings.Contains(re, `"`) {
			return "", fmt.Errorf("a path pattern cannot hold a quote: %s", re)
		}
		w("(deny file-write* (regex #\"%s\"))", re)
	}
	// Renaming a folder needs the right to unlink it, and only the folder itself
	// is named: what is inside stays writable.
	if len(c.NoMove) > 0 {
		w("(deny file-write-unlink %s)", paths("literal", c.NoMove))
	}
	for _, re := range c.NoMovePatterns {
		if strings.Contains(re, `"`) {
			return "", fmt.Errorf("a path pattern cannot hold a quote: %s", re)
		}
		w("(deny file-write-unlink (regex #\"%s\"))", re)
	}

	readable := append(append(append([]string{}, c.Write...), c.Scratch...), c.Read...)
	w("(deny file-read* (subpath %s))", quote(c.Home))
	w("(allow file-read* (literal %s) %s)", quote(c.Home), paths("subpath", readable))
	// The folders above each of them can be looked at but not listed: a path
	// through ~/Documents has to resolve without showing what else is there.
	if up := ancestors(c.Home, readable); len(up) > 0 {
		w("(allow file-read-metadata %s)", paths("literal", up))
	}
	if len(c.NoAccess) > 0 {
		w("(deny file-read* file-write* %s)", paths("subpath", c.NoAccess))
	}
	for _, re := range c.NoAccessPatterns {
		if strings.Contains(re, `"`) {
			return "", fmt.Errorf("a path pattern cannot hold a quote: %s", re)
		}
		w("(deny file-read* file-write* (regex #\"%s\"))", re)
	}
	if len(c.ReadAlways) > 0 {
		w("(allow file-read* %s)", paths("subpath", c.ReadAlways))
	}
	return b.String(), nil
}

// ancestors lists the folders between home and each path under it.
func ancestors(home string, ps []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range ps {
		for d := filepath.Dir(p); strings.HasPrefix(d, home+string(filepath.Separator)); d = filepath.Dir(d) {
			if !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	return out
}

func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
