package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
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

// Go keeps X-Api-Key on a redirect to another host, and a 307 sends the prompt too.
func TestARedirectDoesNotCarryAKey(t *testing.T) {
	for _, typ := range []string{"anthropic", "gemini", "openai_compat", "openai_responses"} {
		t.Run(typ, func(t *testing.T) {
			evil, evilGot := keyTrap(t, testKey)
			target := strings.Replace(evil.URL, "127.0.0.1", "localhost", 1)
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target+r.URL.Path, http.StatusTemporaryRedirect)
			}))
			defer gateway.Close()
			e := openBinding(t, t.TempDir(), secrets.NewMemory())
			pi, err := e.SaveProvider(SaveProviderRequest{Name: "p", Type: typ, Preset: "custom", BaseURL: gateway.URL, APIKey: testKey, DefaultModel: "m"})
			if err != nil {
				t.Fatal(err)
			}
			e.TestProvider(context.Background(), pi.ID)
			if evilGot.Load() {
				t.Error("the key followed a redirect to another host")
			}
		})
	}
}

func TestAddressesCannotHoldCredentials(t *testing.T) {
	e := openBinding(t, t.TempDir(), secrets.NewMemory())
	for _, u := range []string{"https://u:pw@llm.example.com/v1", "https://llm.example.com/v1?api-key=abc", "https://llm.example.com/v1?token=abc"} {
		if _, err := e.SaveProvider(SaveProviderRequest{Name: "p", Type: "openai_compat", Preset: "custom", BaseURL: u}); err == nil {
			t.Errorf("%s was accepted", u)
		}
	}
	if _, err := e.SaveProvider(SaveProviderRequest{Name: "p", Type: "openai_compat", Preset: "custom", BaseURL: "https://llm.example.com/v1?api-version=2"}); err != nil {
		t.Errorf("an ordinary query: %v", err)
	}
	cfg := e.Config()
	cfg.Web.SearchURL = "https://s.example/?key=AIzaFAKE&q={query}"
	if _, err := e.SaveSettings(cfg); err == nil {
		t.Error("a key in the search address was saved")
	}
	cfg.Web.SearchURL = "https://s.example/?key={key}&q={query}"
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Errorf("the {key} placeholder: %v", err)
	}
}

func TestTheSearchKeyInTheAddressStaysOutOfErrors(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	u := dead.URL
	dead.Close()
	e := openBinding(t, t.TempDir(), secrets.NewMemory())
	cfg := e.Config()
	cfg.Web.SearchURL = u + "/s?key={key}&q={query}"
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	if err := e.SetWebSearchKey("AIzaFAKE0123456789"); err != nil {
		t.Fatal(err)
	}
	_, err := sessionWeb{e: e}.Search(context.Background(), "x")
	if err == nil || strings.Contains(err.Error(), "AIzaFAKE") {
		t.Errorf("error = %v", err)
	}
}

// A secret stored for one address did not stay behind when the server moved.
func TestMCPSecretsDoNotFollowANewAddress(t *testing.T) {
	evil, evilGot := keyTrap(t, "ghp_FAKE0123456789abcdefgh")
	honest, _ := keyTrap(t, "ghp_FAKE0123456789abcdefgh")
	mem := secrets.NewMemory()
	e := openBinding(t, t.TempDir(), mem)
	if _, err := e.SaveMCPServer(SaveMCPServerRequest{Server: config.MCPServer{ID: "github", Transport: "http", URL: honest.URL, Enabled: true},
		Secrets: map[string]string{"Authorization": "Bearer ghp_FAKE0123456789abcdefgh"}}); err != nil {
		t.Fatal(err)
	}
	it := e.mcpItem("github", claudeServer{Type: "http", URL: evil.URL}, "/repo/.mcp.json")
	if !strings.Contains(it.Note, "secrets are removed") {
		t.Errorf("the import preview does not say so: %q", it.Note)
	}
	got, err := e.SaveMCPServer(*it.server)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SecretNames) != 0 {
		t.Errorf("secrets kept: %v", got.SecretNames)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	e.ApproveMCPServer(ctx, "github", got.Fingerprint)
	if evilGot.Load() {
		t.Error("a stored token went to the new address")
	}
	if _, err := mem.Get(mcpSecretRef("github", "Authorization")); err == nil {
		t.Error("the secret was left in the Keychain")
	}
	// In the app, changing something else keeps it; changing the address does not.
	if _, err := e.SaveMCPServer(SaveMCPServerRequest{Server: config.MCPServer{ID: "other", Transport: "http", URL: honest.URL}, Secrets: map[string]string{"X": "keep-me-0123456789"}}); err != nil {
		t.Fatal(err)
	}
	kept, err := e.SaveMCPServer(SaveMCPServerRequest{Server: config.MCPServer{ID: "other", Transport: "http", URL: honest.URL, Enabled: true}})
	if err != nil || len(kept.SecretNames) != 1 {
		t.Errorf("an unrelated change: %+v, %v", kept.SecretNames, err)
	}
	moved, err := e.SaveMCPServer(SaveMCPServerRequest{Server: config.MCPServer{ID: "other", Transport: "http", URL: evil.URL, Enabled: true}})
	if err != nil || len(moved.SecretNames) != 0 {
		t.Errorf("a new address kept its secrets: %+v, %v", moved.SecretNames, err)
	}
}

// Keys and tokens are known from the start, in the forms a command might print.
func TestRedaction(t *testing.T) {
	dir, home, mem := t.TempDir(), t.TempDir(), secrets.NewMemory()
	e, err := Open(Options{DataDir: dir, Secrets: mem, Home: home, WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	e.SetWebSearchKey("search-FAKEKEY-0123456789abc")
	e.secrets.Set(oauthRef("srv"), `{"server":"https://x","clientSecret":"client-secret-FAKE-012345","token":{"access_token":"oauth-access-FAKE-0123456789","refresh_token":"oauth-refresh-FAKE-0123456789"}}`)
	e.SaveMCPServer(SaveMCPServerRequest{Server: config.MCPServer{ID: "srv", Transport: "http", URL: "https://x"}})
	e.Close()
	e2 := openBinding(t, dir, mem)
	out := e2.redactText("search-FAKEKEY-0123456789abc oauth-access-FAKE-0123456789 oauth-refresh-FAKE-0123456789 client-secret-FAKE-012345")
	if strings.Contains(out, "FAKE") {
		t.Errorf("not known after a restart: %s", out)
	}
	secret := "p@ss/w+rd=with&sk-audit-FAKE-0123456789"
	e2.remember(secret)
	basic := base64.StdEncoding.EncodeToString([]byte("user:" + secret))
	for name, form := range map[string]string{
		"base64": base64.StdEncoding.EncodeToString([]byte(secret)),
		"url":    url.QueryEscape(secret),
		"path":   url.PathEscape(secret),
		"hex":    hex.EncodeToString([]byte(secret)),
		"HEX":    strings.ToUpper(hex.EncodeToString([]byte(secret))),
		"basic":  "Authorization: Basic " + basic,
		"basic2": base64.StdEncoding.EncodeToString([]byte("ab:" + secret)),
		"basic3": base64.StdEncoding.EncodeToString([]byte("abc:" + secret)),
	} {
		got := e2.redactText(form)
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("%s not redacted: %s", name, got)
		}
	}
	if got := e2.redactText("an ordinary sentence about keys"); got != "an ordinary sentence about keys" {
		t.Errorf("changed ordinary text: %s", got)
	}
}

func TestSkillSourceCannotHoldAToken(t *testing.T) {
	if _, ok := parseGitSource("https://token@github.com/o/r"); ok {
		t.Error("a token in a skill's address was accepted")
	}
	if _, ok := parseGitSource("https://github.com/o/r#main"); !ok {
		t.Error("an ordinary address was refused")
	}
}

// A key an endpoint repeats in its error reached the log unredacted.
func TestAKeyInAnErrorStaysOutOfTheLog(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	e := openBinding(t, t.TempDir(), secrets.NewMemory())
	e.remember(testKey)
	log.Printf("engine: prompt: %s", e.redactText("bad key "+testKey))
	if strings.Contains(buf.String(), testKey) {
		t.Errorf("log = %s", buf.String())
	}
}

func TestBypassIsRefusedInAnUntrustedProject(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	dir := t.TempDir()
	e := openBinding(t, dir, secrets.NewMemory())
	cfg := e.Config()
	cfg.Permissions.AllowBypass = true
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	withScript(e, "s", scripted.Step{Text: "hi"})
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "s", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetMode(ctx, ses.ID, "bypass"); err == nil {
		t.Error("bypass was set in an untrusted project")
	}
	if _, err := e.SetProjectTrust(ctx, repo, true); err != nil {
		t.Fatal(err)
	}
	if err := e.SetMode(ctx, ses.ID, "bypass"); err != nil {
		t.Errorf("in a trusted project: %v", err)
	}
}

// OAuth's exception for a server on this Mac covers that server, not what its
// metadata names.
func TestOAuthPrivateExceptionIsTheServersOwn(t *testing.T) {
	own, _ := keyTrap(t, "unused")
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("reached another private service") }))
	defer other.Close()
	c := oauthClient(own.URL)
	if res, err := c.Get(own.URL); err != nil {
		t.Errorf("the server itself: %v", err)
	} else {
		res.Body.Close()
	}
	if res, err := c.Get(strings.Replace(other.URL, "127.0.0.1", "localhost", 1)); err == nil {
		res.Body.Close()
		t.Error("metadata could point at another service on this Mac")
	}
}

func TestRegistryVersionsArePinned(t *testing.T) {
	for v, want := range map[string]bool{"1.2.3": true, "v1.2.3": true, "0.1.0-beta.1": true, "latest": false, "next": false, "": false} {
		if registryVersion.MatchString(v) != want {
			t.Errorf("%q: %v", v, !want)
		}
	}
}
