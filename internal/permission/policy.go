package permission

import (
	"fmt"
	"os"
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
	// shown is neverRead as written, one spelling each, for the policy page.
	shown []string
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
	}
	// Linux's equivalents: the keyrings, the browsers' profiles with their cookies and saved
	// logins, including the Snap and Flatpak copies, and the password managers' stores.
	// Both sets are always in, whatever the system: a path of the other system's is
	// harmless where it does not exist, and one rule list that does not depend on where
	// it runs is simpler to read, test and hand to a sandbox than two.
	neverRead = append(neverRead,
		filepath.Join(home, ".local", "share", "keyrings"),
		filepath.Join(home, ".local", "share", "kwalletd"),
		filepath.Join(home, ".mozilla"),
		filepath.Join(home, ".config", "google-chrome"),
		filepath.Join(home, ".config", "chromium"),
		filepath.Join(home, ".config", "BraveSoftware"),
		filepath.Join(home, ".config", "microsoft-edge"),
		filepath.Join(home, ".config", "vivaldi"),
		filepath.Join(home, ".config", "opera"),
		filepath.Join(home, "snap", "firefox"),
		filepath.Join(home, "snap", "chromium"),
		filepath.Join(home, ".var", "app", "org.mozilla.firefox"),
		filepath.Join(home, ".var", "app", "com.google.Chrome"),
		filepath.Join(home, ".var", "app", "org.chromium.Chromium"),
		filepath.Join(home, ".pki"),
		filepath.Join(home, ".password-store"),
		filepath.Join(home, ".config", "Bitwarden"),
		filepath.Join(home, ".config", "KeePassXC"),
		appDataDir,
	)
	neverRead = append(neverRead, windowsSecrets(home, os.Getenv("APPDATA"), os.Getenv("LOCALAPPDATA"))...)
	p := &Policy{shown: neverRead}
	for _, d := range neverRead {
		p.neverRead = append(p.neverRead, canonical(d)...)
	}
	p.neverWrite = p.neverRead
	return p
}

// windowsSecrets are Windows' equivalents (NFR-XPLAT-003): the credential and vault
// stores, DPAPI's master keys, which decrypt everything else DPAPI protects, the
// certificates' private keys, the browsers' profiles under AppData, and the tools that
// keep their sign-ins there rather than in a dot folder (.ssh, .aws, .azure, .docker
// and the like are in the home folder on Windows too, and listed above). AppData is
// below the home folder unless an administrator moved it, in which case roaming and
// local, from %APPDATA% and %LOCALAPPDATA%, are added as well. Like Linux's, these are
// in on every system.
func windowsSecrets(home, roaming, local string) []string {
	roamings := []string{filepath.Join(home, "AppData", "Roaming")}
	if roaming != "" && filepath.IsAbs(roaming) && filepath.Clean(roaming) != roamings[0] {
		roamings = append(roamings, filepath.Clean(roaming))
	}
	locals := []string{filepath.Join(home, "AppData", "Local")}
	if local != "" && filepath.IsAbs(local) && filepath.Clean(local) != locals[0] {
		locals = append(locals, filepath.Clean(local))
	}
	out := []string{filepath.Join(home, "_netrc")}
	for _, r := range roamings {
		for _, rel := range [][]string{
			{"Microsoft", "Credentials"}, {"Microsoft", "Protect"}, {"Microsoft", "Vault"},
			{"Microsoft", "Crypto"}, {"Microsoft", "SystemCertificates"},
			{"Mozilla"}, {"Thunderbird"}, {"Opera Software"}, {"Bitwarden"}, {"KeePassXC"}, {"1Password"},
			{"GitHub CLI"}, {"gcloud"}, {"rclone"},
		} {
			out = append(out, filepath.Join(append([]string{r}, rel...)...))
		}
	}
	for _, l := range locals {
		for _, rel := range [][]string{
			{"Microsoft", "Credentials"}, {"Microsoft", "Vault"}, {"Microsoft", "Edge"},
			{"Google", "Chrome"}, {"Chromium"}, {"BraveSoftware"}, {"Vivaldi"}, {"Mozilla"},
			{"1Password"}, {"Bitwarden"},
		} {
			out = append(out, filepath.Join(append([]string{l}, rel...)...))
		}
	}
	return out
}

// NeverRead lists the locations nothing may read, in all their spellings, for a
// sandbox to hold too (ENV-009).
func (p *Policy) NeverRead() []string { return slices.Clone(p.neverRead) }

// Protected lists the locations nothing may read or write, one spelling each (SEC-018).
func (p *Policy) Protected() []string { return slices.Clone(p.shown) }

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
	// What the next session obeys, and what CI runs with the repository's secrets after
	// a push. The SSH sync held these back already; TestActedOnLaterCoversTheSync keeps
	// the two lists one (SEC-005).
	"AGENTS.md", "CLAUDE.md", "GEMINI.md", ".agents", ".github", ".gitlab", ".gitlab-ci.yml", ".circleci", ".travis.yml",
	".gitattributes", ".gitmodules", ".lfsconfig", ".direnv", ".mise", "mise.toml", ".bundle", ".yarn", ".yarnrc",
	".yarnrc.yml", ".pnpmfile.cjs", ".mvn", ".bazelrc", "gradle.properties", ".pylintrc", "pylintrc",
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

// secret reports a path inside the project whose name says it holds secrets, as the
// SSH sync judges it (SEC-005): reading one goes to the model, so the user decides.
func (p *Policy) secret(root string, paths []string) bool {
	roots := canonical(root)
	for _, path := range paths {
		if parts := pathParts(roots, path); len(parts) > 0 && environment.SecretPath(strings.Join(parts, "/")) {
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
