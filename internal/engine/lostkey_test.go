package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// SEC-018: when the Keychain item that signs approval marks is gone, the next
// start does not adopt what the files hold. Everything waits for a yes again,
// whatever a file asks for, and Settings says why.
func TestLostApprovalKeyAdoptsNothing(t *testing.T) {
	dir := t.TempDir()
	marks := t.TempDir()
	old := filepath.Join(marks, "old")
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mkdir := func(name string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	write(config.FileName, "version: 1\nhooks:\n  - event: session_start\n    command: touch "+old+"\n")
	open := func(store secrets.Store) *Engine {
		t.Helper()
		e, err := Open(Options{DataDir: dir, Secrets: store, Home: t.TempDir(), Dev: true, WatchInterval: -1})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}

	// A Mac that has run this version: the hook is adopted and carries a mark.
	e := open(secrets.NewMemory())
	if got := e.PendingCommands(); len(got) != 0 {
		t.Fatalf("the first start did not adopt: %+v", got)
	}
	e.Close()
	if !strings.Contains(read(config.FileName), "commandApprovals") {
		t.Fatal("the adopted hook has no mark in config.yaml")
	}

	// The Keychain is reset (or this is a new computer with the data folder
	// restored), and a file now asks for more than the user ever approved.
	write(config.RulesFileName, "version: 1\nrules:\n  - effect: allow\n    tool: shell\n")
	mkdir(config.EnvironmentsDir)
	write(filepath.Join(config.EnvironmentsDir, "mine.yaml"), "name: Mine\nkind: local\nenv:\n  X: \"1\"\n")
	saved := read(config.FileName)
	if !strings.Contains(saved, "allowBypass: false") {
		t.Fatalf("config.yaml does not say allowBypass: %s", saved)
	}
	write(config.FileName, strings.Replace(saved, "allowBypass: false", "allowBypass: true", 1))

	fresh := secrets.NewMemory()
	e = open(fresh)
	kinds := map[string]bool{}
	for _, c := range e.PendingCommands() {
		kinds[c.Kind] = true
	}
	for _, k := range []string{pendingHook, pendingRule, pendingEnvironment, pendingSetting} {
		if !kinds[k] {
			t.Errorf("a %s from the files was adopted with the new key; pending = %+v", k, e.PendingCommands())
		}
	}
	if _, err := e.usableEnvironment("mine"); err == nil {
		t.Error("an environment from the files can be used without approval")
	}
	if e.bypassRefusal("") == nil {
		t.Error("bypass applies without approval")
	}
	startedSession(t, e, "")
	if exists(old) {
		t.Error("the hook ran with a key the user never approved it under")
	}
	said := false
	for _, is := range e.ConfigIssues() {
		if strings.Contains(is.Message, "key") && strings.Contains(is.Message, "Keychain") {
			said = true
		}
	}
	if !said {
		t.Errorf("Settings does not say why everything waits: %+v", e.ConfigIssues())
	}
	n := len(e.PendingCommands())
	e.Close()

	// The key exists again now; a later start must not take the same chance.
	e = open(fresh)
	defer e.Close()
	if got := e.PendingCommands(); len(got) != n {
		t.Errorf("a later start adopted what waited: %d pending, was %d: %+v", len(got), n, got)
	}
}
