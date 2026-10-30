package engine

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/mcp"
)

// fakeOAuth is an authorization server and an MCP server that wants its
// tokens, on one test server.
type fakeOAuth struct {
	t   *testing.T
	srv *httptest.Server

	mu         sync.Mutex
	valid      map[string]bool
	challenges map[string]string
	registered []string // redirect URIs registered
	refreshes  int
	issued     int
}

func newFakeOAuth(t *testing.T) *fakeOAuth {
	f := &fakeOAuth{t: t, valid: map[string]bool{}, challenges: map[string]string{}}
	server := sdk.NewServer(&sdk.Implementation{Name: "remote", Version: "1"}, nil)
	sdk.AddTool(server, &sdk.Tool{Name: "whoami", Description: "Who am I."},
		func(ctx context.Context, req *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, struct{}, error) {
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "signed in"}}}, struct{}{}, nil
		})
	mcpHandler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil)
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(v)
	}
	prm := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"resource": f.srv.URL + "/mcp", "authorization_servers": []string{f.srv.URL}})
	}
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", prm)
	mux.HandleFunc("/.well-known/oauth-protected-resource", prm)
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/authorize", "token_endpoint": f.srv.URL + "/token",
			"registration_endpoint": f.srv.URL + "/register", "response_types_supported": []string{"code"},
			"grant_types_supported": []string{"authorization_code", "refresh_token"}, "code_challenge_methods_supported": []string{"S256"},
			"token_endpoint_auth_methods_supported": []string{"none"}, "authorization_response_iss_parameter_supported": true,
		})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		var meta map[string]any
		json.NewDecoder(r.Body).Decode(&meta)
		f.mu.Lock()
		for _, u := range meta["redirect_uris"].([]any) {
			f.registered = append(f.registered, u.(string))
		}
		f.mu.Unlock()
		meta["client_id"] = "client-1"
		writeJSON(w, http.StatusCreated, meta)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if f.challenges[r.Form.Get("code")] != base64.RawURLEncoding.EncodeToString(sum[:]) {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			delete(f.challenges, r.Form.Get("code"))
		case "refresh_token":
			if r.Form.Get("refresh_token") != "refresh-1" {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			f.refreshes++
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
			return
		}
		f.issued++
		tok := fmt.Sprintf("access-%d", f.issued)
		f.valid[tok] = true
		writeJSON(w, 200, map[string]any{"access_token": tok, "token_type": "Bearer", "expires_in": 3600, "refresh_token": "refresh-1"})
	})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		ok := f.valid[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
		f.mu.Unlock()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+f.srv.URL+`/.well-known/oauth-protected-resource/mcp"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	})
	f.srv = httptest.NewUnstartedServer(mux)
	if addr := os.Getenv("FREELOOP_OAUTH_TEST_SERVER"); addr != "" {
		// For a look at the MCP page: the fake on a fixed address.
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			panic(err)
		}
		f.srv.Listener = ln
	}
	f.srv.Start()
	return f
}

// browser plays the user's browser: it checks the authorization request,
// "signs in", and follows the redirect back to the app.
func (f *fakeOAuth) browser(opened *int) func(string) error {
	return func(raw string) error {
		*opened++
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		q := u.Query()
		if !strings.HasPrefix(raw, f.srv.URL+"/authorize?") || q.Get("client_id") != "client-1" || q.Get("code_challenge_method") != "S256" ||
			q.Get("response_type") != "code" || q.Get("resource") != f.srv.URL+"/mcp" {
			f.t.Errorf("authorization request = %s", raw)
		}
		code := fmt.Sprintf("code-%d", *opened)
		f.mu.Lock()
		f.challenges[code] = q.Get("code_challenge")
		f.mu.Unlock()
		back := q.Get("redirect_uri") + "?" + url.Values{"code": {code}, "state": {q.Get("state")}, "iss": {f.srv.URL}}.Encode()
		go func() {
			resp, err := http.Get(back)
			if err != nil {
				f.t.Errorf("redirect back: %v", err)
				return
			}
			resp.Body.Close()
		}()
		return nil
	}
}

// MCP-006: a remote server that wants OAuth waits for the user's sign-in
// instead of opening a browser on its own; signing in registers the app,
// runs the authorization code flow with PKCE through the browser and the
// loopback redirect, and keeps the tokens in the Keychain, so a later start
// needs no browser and refreshes an expired token; signing out forgets them.
func TestMCPOAuthSignIn(t *testing.T) {
	ctx := context.Background()
	f := newFakeOAuth(t)
	defer f.srv.Close()
	e := openTest(t, t.TempDir())
	defer e.Close()
	opened := 0
	e.SetOpenURL(f.browser(&opened))

	info, err := e.SaveMCPServer(SaveMCPServerRequest{Server: config.MCPServer{ID: "remote", Name: "Remote", Transport: "http", URL: f.srv.URL + "/mcp", Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	info, err = e.ApproveMCPServer(ctx, "remote", info.Fingerprint)
	if err != nil || info.Status.State != mcp.SignIn || info.SignedIn || opened != 0 {
		t.Fatalf("approved without signing in: %+v, %v, browser opened %d", info.Status, err, opened)
	}

	info, err = e.SignInMCPServer(ctx, "remote")
	if err != nil || info.Status.State != mcp.Running || len(info.Status.Tools) != 1 || !info.SignedIn || info.SignInURL != "" || opened != 1 {
		t.Fatalf("sign in: %+v, %v, browser opened %d", info, err, opened)
	}
	// Only the sign-in registered the app, not the start before it.
	if len(f.registered) != 1 || !strings.HasPrefix(f.registered[0], "http://127.0.0.1:") {
		t.Errorf("registered redirects = %v", f.registered)
	}
	res, err := e.mcp.Call(ctx, "remote", "whoami", nil)
	if err != nil || res.Text != "signed in" {
		t.Fatalf("call = %+v, %v", res, err)
	}
	rec, ok := e.loadOAuth(server(e))
	if !ok || rec.Token.AccessToken != "access-1" || rec.ClientID != "client-1" || rec.TokenURL != f.srv.URL+"/token" {
		t.Fatalf("keychain = %+v", rec)
	}

	// A later start uses the kept tokens, refreshing an expired one.
	e.StopMCPServer("remote")
	rec.Token.Expiry = time.Now().Add(-time.Minute)
	e.saveOAuth("remote", rec)
	if err := e.StartMCPServer(ctx, "remote"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if opened != 1 || f.refreshes != 1 {
		t.Errorf("restart: browser opened %d, refreshes %d", opened, f.refreshes)
	}
	if rec, _ := e.loadOAuth(server(e)); rec.Token.AccessToken != "access-2" {
		t.Errorf("the refreshed token was not kept: %+v", rec.Token)
	}

	// A new URL does not inherit the sign-in: its first request carries no
	// token, and the server waits for a sign-in of its own.
	moved, _ := e.mcpServer("remote")
	moved.URL = f.srv.URL + "/mcp?moved=1"
	info, err = e.SaveMCPServer(SaveMCPServerRequest{Server: moved})
	if err != nil {
		t.Fatal(err)
	}
	if info, err = e.ApproveMCPServer(ctx, "remote", info.Fingerprint); err != nil || info.Status.State != mcp.SignIn || info.SignedIn {
		t.Fatalf("after a URL change: %+v, %v", info.Status, err)
	}
	moved.URL = f.srv.URL + "/mcp"
	info, _ = e.SaveMCPServer(SaveMCPServerRequest{Server: moved})
	e.ApproveMCPServer(ctx, "remote", info.Fingerprint)

	// Signing out forgets the tokens; the next start waits for a sign-in.
	if err := e.SignOutMCPServer("remote"); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.loadOAuth(server(e)); ok {
		t.Error("the sign-in is still in the Keychain")
	}
	if err := e.StartMCPServer(ctx, "remote"); err != nil || e.mcp.Status("remote").State != mcp.SignIn {
		t.Errorf("after sign-out: %v, %+v", err, e.mcp.Status("remote"))
	}
}

// A remote server's sign-in may not reach this Mac or its network; a server
// that is itself on one may.
func TestOAuthClientRefusesPrivateAddresses(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer local.Close()
	if _, err := oauthClient("https://mcp.example.com/mcp").Get(local.URL); err == nil || !strings.Contains(err.Error(), "private address") {
		t.Errorf("a remote server reached %s: %v", local.URL, err)
	}
	byName := strings.Replace(local.URL, "127.0.0.1", "localhost", 1)
	if _, err := oauthClient("https://mcp.example.com/mcp").Get(byName); err == nil || !strings.Contains(err.Error(), "private address") {
		t.Errorf("a remote server reached %s by name: %v", byName, err)
	}
	resp, err := oauthClient(local.URL + "/mcp").Get(local.URL)
	if err != nil {
		t.Fatalf("a local server's own sign-in: %v", err)
	}
	resp.Body.Close()
}

func TestSignInPageScheme(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://auth.example.com/authorize?x=1": true, "http://127.0.0.1:9/authorize": true, "http://localhost/a": true,
		"http://auth.example.com/authorize": false, "file:///etc/passwd": false, "javascript:alert(1)": false, "x-app://open": false, "https:///nohost": false,
	} {
		if got := webPage(raw); got != want {
			t.Errorf("webPage(%q) = %v", raw, got)
		}
	}
}

// server returns the test's "remote" server configuration.
func server(e *Engine) config.MCPServer {
	s, _ := e.mcpServer("remote")
	return s
}

// Signing out while a sign-in waits on the browser ends the wait: nothing
// is kept and the server does not come up afterwards.
func TestSignOutEndsAWaitingSignIn(t *testing.T) {
	ctx := context.Background()
	f := newFakeOAuth(t)
	defer f.srv.Close()
	e := openTest(t, t.TempDir())
	defer e.Close()
	opened := make(chan struct{}, 1)
	e.SetOpenURL(func(string) error { opened <- struct{}{}; return nil }) // the user never finishes
	info, err := e.SaveMCPServer(SaveMCPServerRequest{Server: config.MCPServer{ID: "remote", Name: "Remote", Transport: "http", URL: f.srv.URL + "/mcp", Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	e.ApproveMCPServer(ctx, "remote", info.Fingerprint)
	done := make(chan error, 1)
	go func() { _, err := e.SignInMCPServer(ctx, "remote"); done <- err }()
	select {
	case <-opened:
	case <-time.After(10 * time.Second):
		t.Fatal("the sign-in page did not open")
	}
	if err := e.SignOutMCPServer("remote"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Error("the sign-in succeeded after the sign-out")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the sign-in kept waiting after the sign-out")
	}
	if _, ok := e.loadOAuth(server(e)); ok || e.mcp.Status("remote").State == mcp.Running {
		t.Errorf("after the sign-out: kept %v, %+v", ok, e.mcp.Status("remote"))
	}
}
