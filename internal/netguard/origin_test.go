package netguard

import "testing"

func TestRuntimeCallAllowed(t *testing.T) {
	for _, c := range []struct {
		method, origin, site, host string
		served, want               bool
	}{
		{"POST", "wails://wails.localhost", "", "wails.localhost", false, true},
		{"POST", "wails://wails", "", "", false, true},
		{"POST", "", "", "wails.localhost", false, true},
		{"POST", "http://localhost:34117", "", "localhost:34117", true, true},
		{"POST", "http://localhost:41000", "", "localhost:34117", true, false},
		{"POST", "wails://localhost", "", "localhost:34117", true, false},
		// A page picks the host of what it sends to the app's URL scheme.
		{"POST", "http://localhost:41000", "", "localhost:41000", false, false},
		{"POST", "http://localhost", "", "localhost", false, false},
		{"POST", "http://localhost:41000", "", "wails.localhost", false, false},
		{"POST", "https://evil.example", "", "wails.localhost", false, false},
		{"POST", "null", "", "wails.localhost", false, false},
		{"POST", "", "cross-site", "wails.localhost", false, false},
		{"POST", "", "same-site", "wails.localhost", false, false},
		{"POST", "", "same-origin", "wails.localhost", false, true},
		{"GET", "", "", "wails.localhost", false, false},
		{"GET", "wails://wails.localhost", "", "wails.localhost", false, false},
		{"OPTIONS", "http://localhost:41000", "", "wails.localhost", false, false},
	} {
		if got := RuntimeCallAllowed(c.method, c.origin, c.site, c.host, "client-1", c.served); got != c.want {
			t.Errorf("%s from %q to %q (served %v) = %v", c.method, c.origin, c.host, c.served, got)
		}
	}
	// As WebKit delivered a framed page's no-cors fetch to the packaged
	// app: no origin, and a page cannot add the client header to one.
	if RuntimeCallAllowed("POST", "", "", "localhost", "", false) {
		t.Error("a call without the client header was allowed")
	}
	if !RuntimeCallAllowed("POST", "", "", "localhost", "client-1", false) {
		t.Error("the app's own call was refused")
	}
}
