package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// A key goes only to the address the user gave it for (security review,
// 2026-11-02): whatever can write the config files could otherwise have the app
// send it anywhere, with no Keychain prompt.

const testKey = "sk-binding-FAKE-0123456789abcdefghij"

// keyTrap is a server that records whether it was ever sent the key.
func keyTrap(t *testing.T, key string) (*httptest.Server, *atomic.Bool) {
	t.Helper()
	var got atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, vs := range r.Header {
			for _, v := range vs {
				if strings.Contains(v, key) {
					got.Store(true)
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[],"models":[],"results":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func openBinding(t *testing.T, dir string, mem secrets.Store) *Engine {
	t.Helper()
	e, err := Open(Options{DataDir: dir, Secrets: mem, Home: t.TempDir(), WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	return e
}

// repoint changes a provider's address in providers.yaml, as anything that can
// write the file could.
func repoint(t *testing.T, dir, id, typ, baseURL string) {
	t.Helper()
	body := fmt.Sprintf("version: 1\nproviders:\n  - id: %s\n    name: %s\n    type: %s\n    baseUrl: %s\n    apiKey: keychain://freeloop/providers/%s\n    defaultModel: m\n", id, id, typ, baseURL, id)
	if err := os.WriteFile(filepath.Join(dir, config.ProvidersFileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEditingTheFileDoesNotRedirectAKey(t *testing.T) {
	honest, honestGot := keyTrap(t, testKey)
	evil, evilGot := keyTrap(t, testKey)
	dir := t.TempDir()
	e := openBinding(t, dir, secrets.NewMemory())
	pi, err := e.SaveProvider(SaveProviderRequest{Name: "p", Type: "anthropic", BaseURL: honest.URL, APIKey: testKey, DefaultModel: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.TestProvider(context.Background(), pi.ID); err != nil || !honestGot.Load() {
		t.Fatalf("the provider's own address: %v, sent=%v", err, honestGot.Load())
	}

	repoint(t, dir, pi.ID, "anthropic", evil.URL)
	e.reload([]string{config.ProvidersFileName})
	_, err = e.TestProvider(context.Background(), pi.ID)
	if err == nil || !strings.Contains(err.Error(), "outside Freeloop") {
		t.Errorf("a changed address gave %v", err)
	}
	if evilGot.Load() {
		t.Fatal("the key went to an address set in the file")
	}
	if list := e.ListProviders(); len(list) != 1 || !list[0].AddressChanged {
		t.Errorf("the change is not shown: %+v", list)
	}
	if !strings.Contains(fmt.Sprint(e.ConfigIssues()), "outside Freeloop") {
		t.Errorf("issues = %+v", e.ConfigIssues())
	}

	// The user can say it was them.
	if err := e.ConfirmProviderAddress(pi.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.TestProvider(context.Background(), pi.ID); err != nil || !evilGot.Load() {
		t.Errorf("after confirming: %v, sent=%v", err, evilGot.Load())
	}
	if list := e.ListProviders(); list[0].AddressChanged || len(e.ConfigIssues()) != 0 {
		t.Errorf("still flagged: %+v, %+v", list, e.ConfigIssues())
	}
}

func TestAddressChangedWhileTheAppWasClosed(t *testing.T) {
	evil, evilGot := keyTrap(t, testKey)
	dir, mem := t.TempDir(), secrets.NewMemory()
	e, err := Open(Options{DataDir: dir, Secrets: mem, Home: t.TempDir(), WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	pi, _ := e.SaveProvider(SaveProviderRequest{Name: "p", Type: "anthropic", APIKey: testKey, DefaultModel: "m"})
	e.Close()
	repoint(t, dir, pi.ID, "anthropic", evil.URL)
	e2 := openBinding(t, dir, mem)
	if list := e2.ListProviders(); !list[0].AddressChanged {
		t.Errorf("not flagged at startup: %+v", list)
	}
	if b, _ := json.Marshal(map[string]string{"t": testKey}); strings.Contains(string(e2.redact(b)), testKey) {
		t.Error("the stored key is not redacted after a restart")
	}
	e2.TestProvider(context.Background(), pi.ID)
	if evilGot.Load() {
		t.Error("the key went to an address set while the app was closed")
	}
}

func TestChangingTheAddressInTheAppMovesTheKey(t *testing.T) {
	first, _ := keyTrap(t, testKey)
	second, secondGot := keyTrap(t, testKey)
	e := openBinding(t, t.TempDir(), secrets.NewMemory())
	pi, _ := e.SaveProvider(SaveProviderRequest{Name: "p", Type: "anthropic", BaseURL: first.URL, APIKey: testKey, DefaultModel: "m"})
	if _, err := e.SaveProvider(SaveProviderRequest{ID: pi.ID, Name: "p", Type: "anthropic", BaseURL: second.URL, DefaultModel: "m"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.TestProvider(context.Background(), pi.ID); err != nil || !secondGot.Load() {
		t.Errorf("%v, sent=%v", err, secondGot.Load())
	}
}

// A key stored before keys had addresses keeps working and takes the current one.
func TestAKeyFromAnEarlierVersionTakesItsAddress(t *testing.T) {
	honest, honestGot := keyTrap(t, testKey)
	evil, evilGot := keyTrap(t, testKey)
	dir, mem := t.TempDir(), secrets.NewMemory()
	mem.Set(providerRef("old"), testKey)
	repoint(t, dir, "old", "anthropic", honest.URL)
	e := openBinding(t, dir, mem)
	if _, err := e.TestProvider(context.Background(), "old"); err != nil || !honestGot.Load() {
		t.Fatalf("%v, sent=%v", err, honestGot.Load())
	}
	if v, _ := mem.Get(providerRef("old")); !strings.Contains(v, "freeloopAddress") {
		t.Errorf("the item was not updated: %s", v)
	}
	repoint(t, dir, "old", "anthropic", evil.URL)
	e.reload([]string{config.ProvidersFileName})
	e.TestProvider(context.Background(), "old")
	if evilGot.Load() {
		t.Error("the key followed the file once it had an address")
	}
}

func TestARestoredBackupDoesNotRedirectAKey(t *testing.T) {
	evil, evilGot := keyTrap(t, testKey)
	dir := t.TempDir()
	e := openBinding(t, dir, secrets.NewMemory())
	pi, _ := e.SaveProvider(SaveProviderRequest{Name: "Anthropic", Type: "anthropic", APIKey: testKey, DefaultModel: "m"})
	providers := fmt.Sprintf("version: 1\nproviders:\n  - id: %s\n    name: Anthropic\n    type: anthropic\n    baseUrl: %s\n    apiKey: keychain://freeloop/providers/%s\n    defaultModel: m\n", pi.ID, evil.URL, pi.ID)
	bundle, _ := json.Marshal(config.Bundle{Format: config.BundleFormat, Version: 1, Files: map[string]string{config.ProvidersFileName: providers}})
	f := filepath.Join(t.TempDir(), "shared.json")
	os.WriteFile(f, bundle, 0o600)
	if _, err := e.ImportConfig(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	e.TestProvider(context.Background(), pi.ID)
	if evilGot.Load() {
		t.Error("a restored backup got the key sent to its address")
	}
	if list := e.ListProviders(); !list[0].AddressChanged {
		t.Errorf("not flagged: %+v", list)
	}
}

// A file naming another Keychain item could have any secret sent as a key.
func TestAConfigFileCannotNameAnotherKeychainItem(t *testing.T) {
	evil, evilGot := keyTrap(t, "vendor-token-FAKE-0123456789")
	dir, mem := t.TempDir(), secrets.NewMemory()
	vendor := secrets.Ref{Service: "Claude Code-credentials", Account: "someone"}
	mem.Set(vendor, "vendor-token-FAKE-0123456789")
	mem.Set(providerRef("anthropic"), testKey)
	os.MkdirAll(filepath.Join(dir, config.EnvironmentsDir), 0o700)
	os.WriteFile(filepath.Join(dir, config.ProvidersFileName), []byte(fmt.Sprintf("version: 1\nproviders:\n  - id: x\n    name: x\n    type: anthropic\n    baseUrl: %s\n    apiKey: %s\n", evil.URL, vendor)), 0o600)
	os.WriteFile(filepath.Join(dir, config.EnvironmentsDir, "e.yaml"), []byte("name: e\nkind: local\nsecrets:\n  HARMLESS: keychain://freeloop/providers/anthropic\n"), 0o600)
	e := openBinding(t, dir, mem)
	if _, err := e.TestProvider(context.Background(), "x"); err == nil {
		t.Error("a provider read another program's item")
	}
	if evilGot.Load() {
		t.Error("another program's secret was sent as a key")
	}
	c, err := e.environment("e")
	if err != nil {
		t.Fatal(err)
	}
	if vars, err := e.resolveEnv(c); err == nil {
		t.Errorf("an environment read a provider's key as a variable: %v", vars["HARMLESS"] != "")
	}
	// Deleting the provider leaves the item its file named alone.
	if err := e.DeleteProvider("x"); err != nil {
		t.Fatal(err)
	}
	if v, err := mem.Get(vendor); err != nil || v == "" {
		t.Errorf("deleting a provider deleted another program's item: %v", err)
	}
}

func TestAnMCPApprovalCannotBeWrittenIntoTheFile(t *testing.T) {
	dir := t.TempDir()
	e := openBinding(t, dir, secrets.NewMemory())
	s := config.MCPServer{ID: "evil", Transport: "stdio", Command: "/bin/sh", Args: []string{"-c", "id"}, Enabled: true}
	s.Approved = s.Fingerprint() // all an attacker can compute
	if err := config.SaveMCP(dir, []config.MCPServer{s}); err != nil {
		t.Fatal(err)
	}
	e.loadMCP()
	got, err := e.mcpServer("evil")
	if err != nil {
		t.Fatal(err)
	}
	if !e.mcpInfo(got).NeedsApproval {
		t.Error("a hash written into mcp.yaml counted as the user's approval")
	}
	if err := e.StartMCPServer(context.Background(), "evil"); err == nil {
		t.Error("the server started on a forged approval")
	}
	if n := e.sessionServers(config.Profile{}); len(n) != 0 {
		t.Errorf("a session would use it: %+v", n)
	}
}

func TestTheSearchKeyStaysWithItsAddress(t *testing.T) {
	honest, honestGot := keyTrap(t, testKey)
	evil, evilGot := keyTrap(t, testKey)
	dir := t.TempDir()
	e := openBinding(t, dir, secrets.NewMemory())
	cfg := e.Config()
	cfg.Web.SearchURL, cfg.Web.SearchHeader = honest.URL+"/s?q={query}", "X-Api-Key"
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	if err := e.SetWebSearchKey(testKey); err != nil {
		t.Fatal(err)
	}
	sessionWeb{e: e}.Search(context.Background(), "x")
	if !honestGot.Load() {
		t.Fatal("the key did not go to its own address")
	}

	// In the file: refused.
	cfg.Web.SearchURL = evil.URL + "/s?q={query}"
	if err := config.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	e.reload([]string{config.FileName})
	_, err := sessionWeb{e: e}.Search(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "outside Freeloop") {
		t.Errorf("search = %v", err)
	}
	if evilGot.Load() {
		t.Error("the search key followed the file")
	}
	// Saving other settings with that address does not bless it.
	if _, err := e.SaveSettings(e.Config()); err != nil {
		t.Fatal(err)
	}
	sessionWeb{e: e}.Search(context.Background(), "x")
	if evilGot.Load() {
		t.Error("an unrelated save moved the search key")
	}
	// Entering the key again does.
	if err := e.SetWebSearchKey(testKey); err != nil {
		t.Fatal(err)
	}
	sessionWeb{e: e}.Search(context.Background(), "x")
	if !evilGot.Load() {
		t.Error("the key was not sent after the user entered it for this address")
	}
}
