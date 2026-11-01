package netguard

import "testing"

func TestRuntimeCallAllowed(t *testing.T) {
	for _, c := range []struct {
		method, origin, site, host string
		want                       bool
	}{
		{"POST", "wails://wails.localhost", "", "wails.localhost", true},
		{"POST", "wails://wails", "", "", true},
		{"POST", "", "", "wails.localhost", true},
		{"POST", "http://localhost:34117", "", "localhost:34117", true},
		{"POST", "http://localhost:41000", "", "wails.localhost", false},
		{"POST", "http://localhost:41000", "", "localhost:34117", false},
		{"POST", "https://evil.example", "", "wails.localhost", false},
		{"POST", "null", "", "wails.localhost", false},
		{"POST", "", "cross-site", "wails.localhost", false},
		{"POST", "", "same-site", "wails.localhost", false},
		{"POST", "", "same-origin", "wails.localhost", true},
		{"GET", "", "", "wails.localhost", false},
		{"GET", "wails://wails.localhost", "", "wails.localhost", false},
		{"OPTIONS", "http://localhost:41000", "", "wails.localhost", false},
	} {
		if got := RuntimeCallAllowed(c.method, c.origin, c.site, c.host, "client-1"); got != c.want {
			t.Errorf("%s from %q to %q = %v", c.method, c.origin, c.host, got)
		}
	}
	// As WebKit delivered a framed page's no-cors fetch to the packaged
	// app: no origin, and a page cannot add the client header to one.
	if RuntimeCallAllowed("POST", "", "", "localhost", "") {
		t.Error("a call without the client header was allowed")
	}
	if !RuntimeCallAllowed("POST", "", "", "localhost", "client-1") {
		t.Error("the app's own call was refused")
	}
}
