// Package netguard is the local-only switch (NFR-OFF-003): when it is on,
// every connection the app makes through Go's default HTTP transport (the
// providers, their SDKs and remote MCP servers) may only reach this computer.
// Host names other than localhost are refused without a DNS lookup, so not
// even the name leaves the machine, and proxies are bypassed.
package netguard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	localOnly atomic.Bool
	install   sync.Once

	mu      sync.Mutex
	blocked []Blocked
	notify  func(Blocked)
)

// Blocked is a refused connection.
type Blocked struct {
	Host string    `json:"host"`
	At   time.Time `json:"at"`
}

// maxBlocked bounds the attempts kept.
const maxBlocked = 50

// Install puts the guard into http.DefaultTransport; it is a no-op until
// SetLocalOnly(true). Calling it again does nothing.
func Install() {
	install.Do(func() {
		t, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return
		}
		dial := t.DialContext
		if dial == nil {
			dial = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
		}
		t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			if err := Check(addr); err != nil {
				return nil, err
			}
			if !localOnly.Load() {
				return dial(ctx, network, addr)
			}
			// A name under .localhost goes to the loopback address without
			// a lookup, which could leave the Mac; and whatever a name
			// resolved to, the connection must end up on this computer.
			if host, port, err := net.SplitHostPort(addr); err == nil && strings.HasSuffix(strings.ToLower(strings.TrimSuffix(host, ".")), ".localhost") {
				addr = net.JoinHostPort("127.0.0.1", port)
			}
			c, err := dial(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			if ap, err := netip.ParseAddrPort(c.RemoteAddr().String()); err == nil && !ap.Addr().Unmap().IsLoopback() {
				c.Close()
				return nil, fmt.Errorf("local-only mode blocks the connection to %s: it resolved to %s", addr, ap.Addr())
			}
			return c, nil
		}
		proxy := t.Proxy
		t.Proxy = func(r *http.Request) (*url.URL, error) {
			if localOnly.Load() || proxy == nil {
				return nil, nil
			}
			return proxy(r)
		}
	})
}

// SetLocalOnly turns the switch on or off.
func SetLocalOnly(on bool) { localOnly.Store(on) }

// LocalOnly reports the switch.
func LocalOnly() bool { return localOnly.Load() }

// OnBlocked sets a function told of each refused connection.
func OnBlocked(f func(Blocked)) {
	mu.Lock()
	notify = f
	mu.Unlock()
}

// Check refuses addr (host:port) when the switch is on and it is not this
// Mac, and records the attempt.
func Check(addr string) error {
	if !localOnly.Load() {
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if Local(host) {
		return nil
	}
	b := Blocked{Host: host, At: time.Now().UTC()}
	mu.Lock()
	blocked = append(blocked, b)
	if len(blocked) > maxBlocked {
		blocked = blocked[len(blocked)-maxBlocked:]
	}
	f := notify
	mu.Unlock()
	if f != nil {
		f(b)
	}
	return fmt.Errorf("local-only mode blocks the connection to %s", host)
}

// Local reports whether host is this computer: a loopback address, localhost
// or a name under .localhost.
func Local(host string) bool {
	host = strings.TrimSuffix(strings.Trim(host, "[]"), ".")
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost")
}

// Attempts returns the refused connections, newest last.
func Attempts() []Blocked {
	mu.Lock()
	defer mu.Unlock()
	return append([]Blocked{}, blocked...)
}
