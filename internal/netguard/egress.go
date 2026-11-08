package netguard

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Egress is a sandboxed session's way to the internet (ENV-010). The sandbox
// lets its commands reach only this Mac, and this proxy on this Mac forwards to
// the hosts an environment lists and refuses the rest. So a command that ignores
// the proxy variables gets no connection at all: it fails closed. TLS passes
// through untouched, since the proxy sees only the host a client asks for.
type Egress struct {
	hosts     []string
	token     string
	onBlocked func(host string)
	srv       *http.Server
	addr      string
	// allowPrivate is for tests, whose servers are on loopback.
	allowPrivate bool

	mu   sync.Mutex
	seen map[string]bool
}

// proxyUser names the proxy's only client in its URL.
const proxyUser = "freeloop"

// StartEgress starts a proxy on loopback for the listed hosts. A password in its
// URL keeps other programs on this Mac, sandboxed sessions included, from
// borrowing its list. onBlocked hears once of each host it refuses.
func StartEgress(hosts []string, onBlocked func(host string)) (*Egress, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("the egress proxy: %w", err)
	}
	var b [16]byte
	rand.Read(b[:])
	e := &Egress{hosts: hosts, token: hex.EncodeToString(b[:]), onBlocked: onBlocked, addr: ln.Addr().String(), seen: map[string]bool{}}
	e.srv = &http.Server{Handler: e, ReadHeaderTimeout: 30 * time.Second}
	go e.srv.Serve(ln)
	return e, nil
}

// URL is the proxy's address for a command's environment.
func (e *Egress) URL() string {
	return (&url.URL{Scheme: "http", User: url.UserPassword(proxyUser, e.token), Host: e.addr}).String()
}

// Port is the loopback port the proxy listens on.
func (e *Egress) Port() int {
	_, p, _ := net.SplitHostPort(e.addr)
	port, _ := strconv.Atoi(p)
	return port
}

// ServeUnix serves the proxy on a Unix socket at path too, for a sandbox whose
// commands cannot reach this computer's loopback (bubblewrap's, OQ-037); the
// socket is mounted into it. The password is still needed. Close removes it.
func (e *Egress) ServeUnix(path string) error {
	ln, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("the egress proxy: %w", err)
	}
	go e.srv.Serve(ln)
	return nil
}

// Token is the proxy's password, for redaction.
func (e *Egress) Token() string { return e.token }

// Close stops the proxy and every connection through it.
func (e *Egress) Close() { e.srv.Close() }

func (e *Egress) authorized(r *http.Request) bool {
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(proxyUser+":"+e.token))
	return subtle.ConstantTimeCompare([]byte(r.Header.Get("Proxy-Authorization")), []byte(want)) == 1
}

func (e *Egress) refuse(w http.ResponseWriter, host, why string) {
	e.mu.Lock()
	first := !e.seen[host]
	e.seen[host] = true
	e.mu.Unlock()
	if first && e.onBlocked != nil {
		e.onBlocked(host)
	}
	http.Error(w, why, http.StatusForbidden)
}

func (e *Egress) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !e.authorized(r) {
		w.Header().Set("Proxy-Authenticate", `Basic realm="freeloop"`)
		http.Error(w, "this proxy is for one session", http.StatusProxyAuthRequired)
		return
	}
	host, port, err := target(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !HostAllowed(e.hosts, host, port) {
		e.refuse(w, host, host+" is not on this environment's list of hosts")
		return
	}
	if LocalOnly() {
		e.refuse(w, host, "the app is set to connect to nothing but this Mac")
		return
	}
	if r.Method == http.MethodConnect {
		e.tunnel(w, r, host, port)
		return
	}
	(&httputil.ReverseProxy{
		Rewrite: func(p *httputil.ProxyRequest) {
			p.Out.URL = r.URL
			p.Out.Host = r.Host
			p.Out.Header.Del("Proxy-Authorization")
			p.Out.Header.Del("Proxy-Connection")
		},
		Transport: &http.Transport{DialContext: e.dial, Proxy: nil},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			e.refuse(w, host, err.Error())
		},
	}).ServeHTTP(w, r)
}

// target is the host and port a proxy request is for.
func target(r *http.Request) (string, int, error) {
	authority := r.Host
	scheme := "https"
	if r.Method != http.MethodConnect {
		if r.URL.Scheme != "http" || r.URL.Host == "" {
			return "", 0, errors.New("this proxy takes CONNECT and absolute http URLs")
		}
		authority, scheme = r.URL.Host, "http"
	}
	host, p, err := net.SplitHostPort(authority)
	if err != nil {
		host, p = authority, map[string]string{"https": "443", "http": "80"}[scheme]
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return "", 0, fmt.Errorf("not a port: %q", p)
	}
	return strings.ToLower(strings.TrimSuffix(host, ".")), port, nil
}

// HostAllowed reports whether a list of hosts covers host and port: "example.com"
// the name itself, "*.example.com" names under it. The web's two ports are
// implied, and an entry names any other: "example.com:8443".
func HostAllowed(hosts []string, host string, port int) bool {
	for _, h := range hosts {
		name, p, hasPort := strings.Cut(strings.ToLower(h), ":")
		if hasPort {
			if strconv.Itoa(port) != p {
				continue
			}
		} else if port != 443 && port != 80 {
			continue
		}
		if rest, wild := strings.CutPrefix(name, "*."); wild {
			if strings.HasSuffix(host, "."+rest) {
				return true
			}
		} else if host == name {
			return true
		}
	}
	return false
}

// PrivateAddr reports an address on this Mac or its local network.
func PrivateAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.Is6() {
		// An IPv4 address carried inside an IPv6 one reaches the same place, on a
		// network with NAT64 or 6to4 especially.
		b := ip.As16()
		switch {
		case nat64.Contains(ip), compat4.Contains(ip):
			return PrivateAddr(netip.AddrFrom4([4]byte(b[12:16])))
		case sixToFour.Contains(ip):
			return PrivateAddr(netip.AddrFrom4([4]byte(b[2:6])))
		}
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	for _, p := range privateRanges {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

var (
	nat64     = netip.MustParsePrefix("64:ff9b::/96")
	compat4   = netip.MustParsePrefix("::/96")
	sixToFour = netip.MustParsePrefix("2002::/16")
	// Not public, though Go does not call them private: shared address space,
	// "this network", IETF protocol assignments, benchmarking, and site-local.
	privateRanges = []netip.Prefix{
		netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("fec0::/10"),
	}
)

// dial connects to the address it checked, so a name that resolves to a public
// address for the check cannot then lead to the local network.
func (e *Egress) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	var last error = fmt.Errorf("%s has no address", host)
	for _, ip := range ips {
		if PrivateAddr(ip) && !e.allowPrivate {
			last = fmt.Errorf("%s resolves to a private address", host)
			continue
		}
		c, err := (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return c, nil
		}
		last = err
	}
	return nil, last
}

func (e *Egress) tunnel(w http.ResponseWriter, r *http.Request, host string, port int) {
	upstream, err := e.dial(r.Context(), "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		e.refuse(w, host, err.Error())
		return
	}
	defer upstream.Close()
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "this connection cannot be tunnelled", http.StatusInternalServerError)
		return
	}
	client, buf, err := hj.Hijack()
	if err != nil {
		return
	}
	defer client.Close()
	if _, err := client.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n")); err != nil {
		return
	}
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(upstream, buf)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(client, upstream)
		done <- struct{}{}
	}()
	<-done
}
