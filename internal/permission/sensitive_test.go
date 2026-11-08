package permission

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// SEC-005: what the SSH sync holds back as something this computer acts on, accept-edits
// mode asks about and a sandbox keeps read-only. Two lists drifted apart once: the
// policy lacked AGENTS.md and .github/ while the sync held them.
func TestActedOnLaterCoversTheSync(t *testing.T) {
	// Not files a person or tool acts on later: installed code, which a sandbox
	// would not stop either, git's own folder and this app's, which are protected
	// outright, and what decides what the sync sends.
	notActedOn := []string{"node_modules", ".venv", "venv", "__pycache__", "site-packages", ".git", ".freeloop", ".gitignore", ".ignore", "head"}
	have := map[string]bool{}
	for _, n := range ActedOnLater {
		have[environment.FoldPath(n)] = true
	}
	for _, n := range environment.SyncHeldNames() {
		if !have[environment.FoldPath(n)] && !slices.Contains(notActedOn, n) {
			t.Errorf("the sync holds %s back, and accept-edits mode would write it without asking", n)
		}
	}
}

// SEC-005: a secret's file is read only with the user's say, even inside the project.
func TestReadingASecretAsks(t *testing.T) {
	b, _, _, root := setup(t)
	read := func(name string) Request {
		return Request{Tool: "read_file", Kind: KindRead, Paths: []string{filepath.Join(root, name)}}
	}
	for _, name := range []string{".env", "config/.env.production", "deploy/server.pem", "id_ed25519"} {
		if d := decide(t, b, Context{Mode: ModeAcceptEdits, Root: root}, read(name), "reject_once"); d.Actor != ActorUser {
			t.Errorf("%s was read without asking: %+v", name, d)
		}
	}
	for _, name := range []string{"main.go", ".env.example", "docs/environment.md"} {
		if d := decide(t, b, Context{Mode: ModeAsk, Root: root}, read(name), ""); d.Outcome != Allowed {
			t.Errorf("%s: %+v", name, d)
		}
	}
	// Bypass is bypass.
	if d := decide(t, b, Context{Mode: ModeBypass, Root: root}, read(".env"), ""); d.Outcome != Allowed {
		t.Errorf("bypass: %+v", d)
	}
	// Accept edits asks before writing what the next session obeys.
	write := Request{Tool: "write_file", Kind: KindEdit, Paths: []string{filepath.Join(root, "AGENTS.md")}}
	if d := decide(t, b, Context{Mode: ModeAcceptEdits, Root: root}, write, "reject_once"); d.Actor != ActorUser {
		t.Errorf("AGENTS.md was written without asking: %+v", d)
	}
}
