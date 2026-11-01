package engine

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/netguard"
)

const registryPageJSON = `{"servers":[
 {"server":{"name":"io.github.acme/files","description":"Files","version":"1.2.3","repository":{"url":"https://github.com/acme/files"},
   "packages":[
     {"registryType":"npm","identifier":"@acme/files","version":"1.2.3","transport":{"type":"stdio"},
      "packageArguments":[{"type":"named","name":"--root","value":"."}],
      "environmentVariables":[{"name":"OPTIONAL_LEVEL","default":"info"},{"name":"ACME_KEY","isRequired":true,"isSecret":true,"default":"sk-template"}]},
     {"registryType":"pypi","identifier":"acme-files","version":"1.2.3","transport":{"type":"stdio"}},
     {"registryType":"oci","identifier":"docker.io/acme/files","version":"1.2.3","transport":{"type":"stdio"}},
     {"registryType":"npm","identifier":"--evil=1; rm -rf ~","version":"1.0.0"},
     {"registryType":"npm","identifier":"bad-version","version":"1.0.0; touch x"},
     {"registryType":"npm","identifier":"asks","version":"1.0.0","packageArguments":[{"type":"positional","isRequired":true}]}],
   "remotes":[
     {"type":"streamable-http","url":"https://mcp.acme.example/mcp","headers":[{"name":"Authorization","isSecret":true,"isRequired":true}]},
     {"type":"sse","url":"https://mcp.acme.example/sse"},
     {"type":"streamable-http","url":"http://plain.example/mcp"}]},
  "_meta":{}},
 {"server":{"name":"com.example/none","description":"<b>hi</b>","version":"1","repository":{"url":"javascript:alert(1)"}}}],
 "metadata":{"nextCursor":"abc","count":2}}`

// MCP-009: results become options pinned to their version, what cannot run here
// is named, and hostile values do not become a command line.
func TestSearchMCPRegistry(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	if _, err := e.SearchMCPRegistry(ctx, "files", ""); err == nil || !strings.Contains(err.Error(), "Settings") {
		t.Errorf("with no address: %v", err)
	}
	var query string
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		query = r.URL.Path + "?" + r.URL.RawQuery
		io.WriteString(w, registryPageJSON)
	}))
	defer srv.Close()
	if calls != 0 {
		t.Fatal("connected before an address was set")
	}
	for _, bad := range []string{"http://example.com", "ftp://x", "not a url"} {
		if err := e.SetMCPRegistryAddress(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
	if err := e.SetMCPRegistryAddress(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}
	if e.MCPRegistryAddress() != srv.URL {
		t.Errorf("address = %q", e.MCPRegistryAddress())
	}
	page, err := e.SearchMCPRegistry(ctx, "files", "c1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(query, "/v0.1/servers?") || !strings.Contains(query, "search=files") || !strings.Contains(query, "cursor=c1") || !strings.Contains(query, "version=latest") {
		t.Errorf("query = %s", query)
	}
	if page.Next != "abc" || len(page.Servers) != 2 {
		t.Fatalf("page = %+v", page)
	}
	s := page.Servers[0]
	if len(s.Options) != 3 {
		t.Fatalf("options = %+v", s.Options)
	}
	npm := s.Options[0]
	if npm.Command != "npx" || strings.Join(npm.Args, " ") != "-y @acme/files@1.2.3 --root ." {
		t.Errorf("npm = %+v", npm)
	}
	if npm.Variables[0].Name != "ACME_KEY" || npm.Variables[0].Default != "" || !npm.Variables[0].Secret {
		t.Errorf("a secret's default was kept, or it was not first: %+v", npm.Variables)
	}
	if py := s.Options[1]; py.Command != "uvx" || py.Args[0] != "acme-files==1.2.3" {
		t.Errorf("pypi = %+v", py)
	}
	if r := s.Options[2]; r.Transport != "http" || r.URL != "https://mcp.acme.example/mcp" || r.Headers[0].Name != "Authorization" {
		t.Errorf("remote = %+v", r)
	}
	if len(s.Unsupported) != 6 {
		t.Errorf("unsupported = %v", s.Unsupported)
	}
	for _, o := range s.Options {
		if strings.Contains(strings.Join(o.Args, " "), "rm -rf") || strings.Contains(strings.Join(o.Args, " "), "touch") {
			t.Errorf("a hostile value became an argument: %+v", o)
		}
	}
	if page.Servers[1].Repository != "" {
		t.Errorf("a script address was kept as a link: %q", page.Servers[1].Repository)
	}
	netguard.SetLocalOnly(true)
	defer netguard.SetLocalOnly(false)
	if _, err := e.SearchMCPRegistry(ctx, "", ""); err == nil {
		t.Error("searched in local-only mode")
	}
}
