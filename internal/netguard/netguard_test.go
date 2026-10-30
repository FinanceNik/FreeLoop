package netguard

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// NFR-OFF-003: with the switch on, a request to another host fails before
// any lookup and is recorded; this Mac stays reachable; off, nothing
// changes.
func TestLocalOnly(t *testing.T) {
	Install()
	defer SetLocalOnly(false)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	defer srv.Close()

	SetLocalOnly(true)
	var told []string
	OnBlocked(func(b Blocked) { told = append(told, b.Host) })
	if _, err := http.Get("https://api.example.invalid/v1/models"); err == nil || !strings.Contains(err.Error(), "local-only mode blocks the connection to api.example.invalid") {
		t.Errorf("err = %v", err)
	}
	if _, err := http.Get("http://203.0.113.7:8080/"); err == nil {
		t.Error("an IP address outside reached")
	}
	resp, err := http.Get(srv.URL)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("loopback: %v", err)
	}
	resp.Body.Close()
	if a := Attempts(); len(a) != 2 || len(told) != 2 || told[0] != "api.example.invalid" {
		t.Errorf("attempts = %+v, told = %v", a, told)
	}
	for host, want := range map[string]bool{"localhost": true, "LOCALHOST": true, "ollama.localhost": true, "127.0.0.1": true, "::1": true, "[::1]": true, "10.0.0.1": false, "example.com": false, "localhost.example.com": false} {
		if Local(host) != want {
			t.Errorf("Local(%q) = %v", host, !want)
		}
	}

	SetLocalOnly(false)
	if err := Check("example.com:443"); err != nil {
		t.Errorf("off: %v", err)
	}
}
