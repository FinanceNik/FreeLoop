package netguard

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestHostAllowed(t *testing.T) {
	list := []string{"registry.npmjs.org", "*.github.com", "git.example.com:8443"}
	for _, c := range []struct {
		host string
		port int
		want bool
	}{
		{"registry.npmjs.org", 443, true},
		{"registry.npmjs.org", 80, true},
		{"registry.npmjs.org", 22, false},
		{"evil-registry.npmjs.org", 443, false},
		{"registry.npmjs.org.evil.com", 443, false},
		{"api.github.com", 443, true},
		{"a.b.github.com", 443, true},
		{"github.com", 443, false},
		{"notgithub.com", 443, false},
		{"git.example.com", 8443, true},
		{"git.example.com", 443, false},
		{"example.com", 443, false},
	} {
		if got := HostAllowed(list, c.host, c.port); got != c.want {
			t.Errorf("%s:%d = %v", c.host, c.port, got)
		}
	}
}

// proxied starts a proxy for hosts and a client that uses it.
func proxied(t *testing.T, hosts []string, allowPrivate bool) (*Egress, *http.Client, *[]string) {
	t.Helper()
	var mu sync.Mutex
	blocked := &[]string{}
	e, err := StartEgress(hosts, func(h string) {
		mu.Lock()
		*blocked = append(*blocked, h)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	e.allowPrivate = allowPrivate
	t.Cleanup(e.Close)
	u, _ := url.Parse(e.URL())
	return e, &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(u)}}, blocked
}

// ENV-010: a listed host is reached, in plain HTTP and through a tunnel, and any
// other is refused and reported once.
func TestEgress(t *testing.T) {
	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Proxy-Authorization")
		io.WriteString(w, "hello")
	}))
	defer srv.Close()
	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	_, client, blocked := proxied(t, []string{host + ":" + port}, true)

	res, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(body) != "hello" || seenAuth != "" {
		t.Errorf("body %q, the password reached the server: %q", body, seenAuth)
	}
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "secure") }))
	defer tls.Close()
	h2, p2, _ := net.SplitHostPort(tls.Listener.Addr().String())
	e2, c2, _ := proxied(t, []string{h2 + ":" + p2}, true)
	_ = e2
	c2.Transport.(*http.Transport).TLSClientConfig = tls.Client().Transport.(*http.Transport).TLSClientConfig
	res, err = c2.Get(tls.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if string(body) != "secure" {
		t.Errorf("through the tunnel: %q", body)
	}

	for range 2 {
		if res, err := client.Get("http://example.invalid/"); err == nil {
			if res.StatusCode != http.StatusForbidden {
				t.Errorf("an unlisted host: %s", res.Status)
			}
			res.Body.Close()
		}
	}
	if _, err := client.Get("https://example.invalid/"); err == nil {
		t.Error("a tunnel to an unlisted host opened")
	}
	if fmt.Sprint(*blocked) != "[example.invalid]" {
		t.Errorf("blocked = %v", *blocked)
	}
}

// ENV-010: a listed name that leads to this Mac or its network is refused, so a
// list cannot be turned on local services by changing what a name resolves to.
func TestEgressRefusesPrivateAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "local") }))
	defer srv.Close()
	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	_, client, _ := proxied(t, []string{host + ":" + port, "localhost:" + port}, false)
	for _, u := range []string{srv.URL, "http://localhost:" + port} {
		res, err := client.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden || strings.Contains(string(body), "local\n") && string(body) == "local" {
			t.Errorf("%s: %s %q", u, res.Status, body)
		}
	}
}

// Other programs cannot borrow the list, and local-only mode closes it.
func TestEgressNeedsItsPassword(t *testing.T) {
	e, client, _ := proxied(t, []string{"example.invalid"}, true)
	u, _ := url.Parse(e.URL())
	u.User = url.UserPassword(proxyUser, "wrong")
	wrong := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(u)}}
	res, err := wrong.Get("http://example.invalid/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusProxyAuthRequired {
		t.Errorf("a wrong password: %s", res.Status)
	}
	SetLocalOnly(true)
	defer SetLocalOnly(false)
	res, err = client.Get("http://example.invalid/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("in local-only mode: %s", res.Status)
	}
}
