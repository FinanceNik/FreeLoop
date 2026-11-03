package permission

import (
	"fmt"
	"path/filepath"
	"slices"
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
		// Other programs' sign-ins. HRN-008: the app never reads these, and an
		// "allow always" must not cover them. A vendor's own agent reads its own
		// itself, not through here.
		filepath.Join(home, ".codex", "auth.json"),
		filepath.Join(home, ".gemini", "oauth_creds.json"),
		filepath.Join(home, ".gemini", "google_accounts.json"),
		filepath.Join(home, ".claude", ".credentials.json"),
		filepath.Join(home, ".claude.json"),
		filepath.Join(home, ".netrc"),
		filepath.Join(home, ".npmrc"),
		filepath.Join(home, ".pypirc"),
		filepath.Join(home, ".cargo", "credentials"),
		filepath.Join(home, ".cargo", "credentials.toml"),
		filepath.Join(home, ".git-credentials"),
		filepath.Join(home, ".config", "gh"),
		filepath.Join(home, ".config", "gcloud"),
		filepath.Join(home, ".docker", "config.json"),
		filepath.Join(home, ".kube"),
		filepath.Join(home, ".azure"),
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

// NeverRead lists the locations nothing may read, in all their spellings, for a
// sandbox to hold too (ENV-009).
func (p *Policy) NeverRead() []string { return slices.Clone(p.neverRead) }

// canonical returns the spellings of p that checks must cover.
func canonical(p string) []string { return environment.Spellings(p) }

// within reports whether p is dir or inside it, spelled as macOS volumes
// compare names, so ~/.SSH is ~/.ssh and .GIT is .git (environment.FoldPath).
func within(p, dir string) bool {
	p, dir = environment.FoldPath(p), environment.FoldPath(dir)
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
	// At any depth: a nested repository's config runs when git starts in it, and
	// the project's Freeloop settings can add hooks, which run commands; only the
	// user edits them (EXT-005, SEC-006). A nested .freeloop is the settings of a
	// project opened there.
	gitDir, settings := environment.FoldPath(".git"), environment.FoldPath(ProjectSettingsDir)
	for _, part := range pathParts(roots, path) {
		switch environment.FoldPath(part) {
		case gitDir:
			return fmt.Sprintf("%s is inside .git", path)
		case settings:
			return fmt.Sprintf("%s is inside %s, the project's Freeloop settings, which only you edit", path, ProjectSettingsDir)
		}
	}
	return ""
}

// pathParts returns path's names below whichever root it is in.
func pathParts(roots []string, path string) []string {
	fp := environment.FoldPath(path)
	for _, r := range roots {
		fr := environment.FoldPath(r)
		if strings.HasPrefix(fp, fr+string(filepath.Separator)) {
			return strings.Split(fp[len(fr)+1:], string(filepath.Separator))
		}
	}
	return nil
}

// ActedOnLater are the project files something outside a session later reads and
// runs: editors, direnv, commit hooks and other agents. Writing one is not an
// ordinary edit, so accept-edits mode asks, and a sandbox keeps them read-only.
var ActedOnLater = []string{
	".devcontainer", ".devcontainer.json", ".mcp.json", ".envrc", ".vscode", ".idea", ".husky", ".githooks",
	".pre-commit-config.yaml", ".claude", ".gemini", ".codex", ".goose", ".opencode", "opencode.json", ".cursor",
	".windsurf", ".continue", ".zed", ".mise.toml", ".tool-versions", ".nvim.lua", ".nvimrc", ".exrc", ".cargo",
}

// actedOn reports a path that is or is inside one of ActedOnLater.
func (p *Policy) actedOn(root string, paths []string) bool {
	roots := canonical(root)
	for _, path := range paths {
		parts := pathParts(roots, path)
		if len(parts) == 0 {
			continue
		}
		// At any depth: opening a subfolder as a project makes its files active.
		for _, name := range ActedOnLater {
			if slices.Contains(parts, environment.FoldPath(name)) {
				return true
			}
		}
		// direnv looks in every folder on the way.
		if parts[len(parts)-1] == environment.FoldPath(".envrc") {
			return true
		}
	}
	return false
}

// ProjectSettingsDir is a project's Freeloop settings folder; harnesses may
// read it but never write it.
const ProjectSettingsDir = ".freeloop"

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
