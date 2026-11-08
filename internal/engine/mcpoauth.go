package engine

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/mcp"
	"github.com/FinanceNik/Freeloop/internal/netguard"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// OAuth for remote MCP servers (MCP-006). The MCP SDK runs the protocol:
// protected resource and authorization server metadata, client
// registration (a pre-registered client ID when the server's entry names
// one, else Dynamic Client Registration), the authorization code flow with
// PKCE, the issuer check and token refresh. The app adds the loopback
// redirect, the sign-in page in the user's browser, and the Keychain: one
// item per server holds the client and the tokens, so a restart does not
// ask again. A background start never opens a browser; the server waits
// in the sign_in state for the user's "Sign in".

// signInTimeout bounds how long a sign-in waits for the browser.
const signInTimeout = 5 * time.Minute

type interactiveKey struct{}

// signingIn marks a server start the user asked to sign in for.
func signingIn(ctx context.Context) context.Context {
	return context.WithValue(ctx, interactiveKey{}, true)
}

func isSigningIn(ctx context.Context) bool {
	v, _ := ctx.Value(interactiveKey{}).(bool)
	return v
}

// oauthRecord is what the Keychain keeps for a server.
type oauthRecord struct {
	// Server is the URL the sign-in was for; a server whose URL changed
	// signs in again rather than hand its tokens to the new address.
	Server       string           `json:"server"`
	ClientID     string           `json:"clientId"`
	ClientSecret string           `json:"clientSecret,omitempty"`
	AuthURL      string           `json:"authUrl"`
	TokenURL     string           `json:"tokenUrl"`
	AuthStyle    oauth2.AuthStyle `json:"authStyle"`
	Scopes       []string         `json:"scopes,omitempty"`
	Token        *oauth2.Token    `json:"token"`
}

func (r oauthRecord) config() *oauth2.Config {
	return &oauth2.Config{
		ClientID: r.ClientID, ClientSecret: r.ClientSecret, Scopes: r.Scopes,
		Endpoint: oauth2.Endpoint{AuthURL: r.AuthURL, TokenURL: r.TokenURL, AuthStyle: r.AuthStyle},
	}
}

func oauthRef(serverID string) secrets.Ref {
	return secrets.Ref{Service: "freeloop", Account: "mcp-oauth/" + serverID}
}

func (e *Engine) loadOAuth(s config.MCPServer) (oauthRecord, bool) {
	v, err := e.secrets.Get(oauthRef(s.ID))
	if err != nil {
		return oauthRecord{}, false
	}
	var r oauthRecord
	if json.Unmarshal([]byte(v), &r) != nil || r.Token == nil {
		return oauthRecord{}, false
	}
	if r.Server != s.URL {
		e.secrets.Delete(oauthRef(s.ID))
		return oauthRecord{}, false
	}
	return r, true
}

// oauthSecrets are the secret values in a stored sign-in, for redaction.
func oauthSecrets(stored string) []string {
	var r oauthRecord
	if json.Unmarshal([]byte(stored), &r) != nil {
		return nil
	}
	out := []string{r.ClientSecret}
	if r.Token != nil {
		out = append(out, r.Token.AccessToken, r.Token.RefreshToken)
	}
	return out
}

func (e *Engine) saveOAuth(serverID string, r oauthRecord) {
	b, _ := json.Marshal(r)
	if err := e.secrets.Set(oauthRef(serverID), string(b)); err != nil {
		log.Printf("mcp %s: keep the sign-in in the Keychain: %v", serverID, err)
	}
	// From now, not only from the next start: a registration's client secret too.
	for _, secret := range oauthSecrets(string(b)) {
		e.remember(secret)
	}
}

// savingSource hands out a server's tokens, refreshing them when they
// expire, and keeps each new one in the Keychain.
type savingSource struct {
	e   *Engine
	id  string
	src oauth2.TokenSource

	mu   sync.Mutex
	rec  oauthRecord
	last string
}

func (s *savingSource) Token() (*oauth2.Token, error) {
	t, err := s.src.Token()
	if err != nil {
		return nil, fmt.Errorf("refresh the sign-in: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.AccessToken != s.last {
		s.last = t.AccessToken
		s.rec.Token = t
		s.e.saveOAuth(s.id, s.rec)
	}
	return t, nil
}

// oauthHandler returns the OAuth handler for an HTTP server: the tokens
// from an earlier sign-in, or none until the user signs in.
func (e *Engine) oauthHandler(s config.MCPServer) (auth.OAuthHandler, error) {
	port, err := freeLoopbackPort()
	if err != nil {
		return nil, err
	}
	redirect := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	client := oauthClient(s.URL)
	saving := func(ctx context.Context, rec oauthRecord) oauth2.TokenSource {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, client)
		return &savingSource{e: e, id: s.ID, rec: rec, src: rec.config().TokenSource(ctx, rec.Token)}
	}
	cfg := &auth.AuthorizationCodeHandlerConfig{
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{
			ClientName:              "Freeloop",
			ClientURI:               "https://github.com/FinanceNik/Freeloop",
			RedirectURIs:            []string{redirect},
			GrantTypes:              []string{"authorization_code", "refresh_token"},
			ResponseTypes:           []string{"code"},
			TokenEndpointAuthMethod: "none",
		}},
		RedirectURL:              redirect,
		AuthorizationCodeFetcher: e.fetchCode(s.ID, port),
		RequestRefreshToken:      true,
		Client:                   client,
		NewTokenSource: func(ctx context.Context, c *oauth2.Config, t *oauth2.Token) (oauth2.TokenSource, error) {
			rec := oauthRecord{Server: s.URL, ClientID: c.ClientID, ClientSecret: c.ClientSecret, AuthURL: c.Endpoint.AuthURL, TokenURL: c.Endpoint.TokenURL,
				AuthStyle: c.Endpoint.AuthStyle, Scopes: c.Scopes, Token: t}
			e.saveOAuth(s.ID, rec)
			return saving(context.Background(), rec), nil
		},
	}
	if s.OAuthClientID != "" {
		cfg.PreregisteredClient = &oauthex.ClientCredentials{ClientID: s.OAuthClientID}
	}
	if rec, ok := e.loadOAuth(s); ok {
		e.remember(rec.Token.AccessToken)
		e.remember(rec.Token.RefreshToken)
		cfg.InitialTokenSource = saving(context.Background(), rec)
	}
	h, err := auth.NewAuthorizationCodeHandler(cfg)
	if err != nil {
		return nil, err
	}
	return signInGate{h}, nil
}

// signInGate lets the authorization flow run only when the user asked to
// sign in: otherwise not even a client registration goes out.
type signInGate struct{ auth.OAuthHandler }

func (g signInGate) Authorize(ctx context.Context, req *http.Request, resp *http.Response) error {
	if !isSigningIn(ctx) {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return mcp.ErrSignIn
	}
	return g.OAuthHandler.Authorize(ctx, req, resp)
}

// fetchCode shows the sign-in page in the user's browser and waits for the
// authorization server to redirect back to the loopback port.
func (e *Engine) fetchCode(serverID string, port int) auth.AuthorizationCodeFetcher {
	return func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
		// The page comes from the server's metadata: only a web page
		// opens, never another scheme.
		if !webPage(args.URL) {
			return nil, fmt.Errorf("the sign-in page %q is not an https address", args.URL)
		}
		// The state the SDK put in the sign-in page's address: only a
		// redirect that carries it back ends the wait.
		u, _ := url.Parse(args.URL)
		want := u.Query().Get("state")
		if want == "" {
			return nil, errors.New("the sign-in page carries no state")
		}
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return nil, fmt.Errorf("listen for the sign-in: %w", err)
		}
		got := make(chan url.Values, 1)
		srv := &http.Server{
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       10 * time.Second,
			MaxHeaderBytes:    16 << 10,
			Handler:           callbackHandler(want, got),
			ErrorLog:          log.New(io.Discard, "", 0),
		}
		go srv.Serve(ln)
		defer srv.Close()

		e.setSignInURL(serverID, args.URL)
		defer e.setSignInURL(serverID, "")
		if e.openURL != nil {
			if err := e.openURL(args.URL); err != nil {
				log.Printf("mcp %s: open the sign-in page: %v", serverID, err)
			}
		}
		select {
		case q := <-got:
			if errText := q.Get("error"); errText != "" {
				if d := q.Get("error_description"); d != "" {
					errText += ": " + d
				}
				return nil, fmt.Errorf("the sign-in was refused: %s", errText)
			}
			return &auth.AuthorizationResult{Code: q.Get("code"), State: q.Get("state"), Iss: q.Get("iss")}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(signInTimeout):
			return nil, errors.New("the sign-in was not finished in the browser within 5 minutes")
		}
	}
}

// callbackHandler answers the loopback redirect of a sign-in. Any page in
// the browser and any local process can reach the address, so a request
// that is not a GET of /callback carrying the sign-in's state, and a code
// or an error, gets a plain 400 and is otherwise ignored: the sign-in keeps
// waiting for the real redirect. Only the right state ends it, with the
// code or as a failure with the authorization server's error.
func callbackHandler(want string, got chan<- url.Values) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 0)
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/callback" ||
			subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(want)) != 1 ||
			(q.Get("code") == "" && q.Get("error") == "") {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		msg := "Signed in. You can close this tab and return to Freeloop."
		if q.Get("error") != "" {
			msg = "The sign-in did not complete: " + q.Get("error") + ". Return to Freeloop to try again."
		}
		fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>Freeloop</title><p style=\"font:15px system-ui;margin:3em\">%s</p>", html.EscapeString(msg))
		select {
		case got <- q:
		default:
		}
	})
}

func (e *Engine) setSignInURL(serverID, u string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.signIns == nil {
		e.signIns = map[string]string{}
	}
	if u == "" {
		delete(e.signIns, serverID)
	} else {
		e.signIns[serverID] = u
	}
}

func (e *Engine) signInURL(serverID string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.signIns[serverID]
}

// SignInMCPServer starts a remote server and runs its sign-in in the
// user's browser when the server asks for one.
func (e *Engine) SignInMCPServer(ctx context.Context, id string) (MCPServerInfo, error) {
	ctx, cancel := context.WithTimeout(signingIn(ctx), signInTimeout+time.Minute)
	defer cancel()
	err := e.StartMCPServer(ctx, id)
	s, _ := e.mcpServer(id)
	return e.mcpInfo(s), err
}

// SignOutMCPServer forgets a server's sign-in and stops it.
func (e *Engine) SignOutMCPServer(id string) error {
	e.mcp.Stop(id)
	if err := e.secrets.Delete(oauthRef(id)); err != nil && !errors.Is(err, secrets.ErrNotFound) {
		return err
	}
	return nil
}

func freeLoopbackPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// oauthClient is the HTTP client for a server's OAuth requests. It goes
// through http.DefaultTransport, so the local-only switch covers it
// (NFR-OFF-003), and refuses private and loopback addresses unless the
// server itself is on one (by address or by what its name resolves to), so
// a server's metadata cannot point the app at services on this computer or its
// network. Every request's host is checked before it is sent, proxied ones
// too, and a direct connection's address once more after it is made.
func oauthClient(serverURL string) *http.Client {
	u, _ := url.Parse(serverURL)
	if u != nil && hostIsPrivate(u.Hostname()) {
		// Only the server itself: its metadata still cannot point at anything
		// else on this computer or its network, whatever its name resolves to later.
		return &http.Client{Timeout: 30 * time.Second, Transport: ownHostOnly{host: u.Host, own: http.DefaultTransport, others: publicClient().Transport}}
	}
	return publicClient()
}

// ownHostOnly sends requests to one host directly and the rest through others.
type ownHostOnly struct {
	host        string
	own, others http.RoundTripper
}

func (t ownHostOnly) RoundTrip(r *http.Request) (*http.Response, error) {
	if strings.EqualFold(r.URL.Host, t.host) {
		return t.own.RoundTrip(r)
	}
	return t.others.RoundTrip(r)
}

// publicClient is an HTTP client that reaches public addresses only: every
// request's host is checked before it is sent, each redirect's too, and a
// direct connection's address once more after it is made, so a name that
// resolves differently the second time gets no further. It goes through
// http.DefaultTransport, so the local-only switch covers it.
func publicClient() *http.Client {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Client{Transport: publicOnly{http.DefaultTransport}, Timeout: 30 * time.Second}
	}
	t := base.Clone()
	dial := t.DialContext
	if dial == nil {
		dial = (&net.Dialer{Timeout: 30 * time.Second}).DialContext
	}
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		c, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		// Through a proxy the connection is the proxy's; the request's
		// host was checked before it was sent.
		if t.Proxy != nil {
			if p, perr := t.Proxy(&http.Request{URL: &url.URL{Scheme: "https", Host: addr}}); perr == nil && p != nil {
				return c, nil
			}
		}
		if ap, err := netip.ParseAddrPort(c.RemoteAddr().String()); err == nil && netguard.PrivateAddr(ap.Addr()) {
			c.Close()
			return nil, fmt.Errorf("refused %s: this request may not reach a private address", addr)
		}
		return c, nil
	}
	return &http.Client{Transport: publicOnly{t}, Timeout: 30 * time.Second}
}

// publicOnly refuses a request whose host is private or resolves to a
// private address.
type publicOnly struct{ next http.RoundTripper }

func (p publicOnly) RoundTrip(r *http.Request) (*http.Response, error) {
	if hostIsPrivate(r.URL.Hostname()) {
		return nil, fmt.Errorf("refused %s: this request may not reach a private address", r.URL.Host)
	}
	return p.next.RoundTrip(r)
}

// hostIsPrivate reports a host that is, or whose name resolves to, a
// private or loopback address. In local-only mode no name is looked up:
// the connection is refused anyway, and the name must not leave the Mac.
func hostIsPrivate(host string) bool {
	if privateHost(host) {
		return true
	}
	if _, err := netip.ParseAddr(host); err == nil || netguard.LocalOnly() {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if netguard.PrivateAddr(ip) {
			return true
		}
	}
	return false
}

func privateHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && netguard.PrivateAddr(ip)
}

// webPage reports an https address, or an http one on this computer.
func webPage(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme == "https" {
		return u.Host != ""
	}
	if u.Scheme != "http" {
		return false
	}
	if u.Hostname() == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(u.Hostname())
	return err == nil && ip.Unmap().IsLoopback()
}
