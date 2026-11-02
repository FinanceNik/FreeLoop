// Package mcp runs the app's MCP servers on the official Go SDK (MCP-001):
// one client per configured server, over stdio (a process the environment
// spawns) or Streamable HTTP. The SDK negotiates the protocol revision,
// using server/discover for 2026-07-28 servers and initialize for older
// ones. The package starts no process and touches no file itself.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
)

// State is a server's lifecycle state.
type State string

const (
	Stopped  State = "stopped"
	Starting State = "starting"
	Running  State = "running"
	Failed   State = "error"
	// SignIn: the server wants the user to sign in (MCP-006).
	SignIn State = "sign_in"
)

// ConnectTimeout bounds starting a server and listing its tools; MCP-001
// asks for tools within 5 s, local package installs can take longer.
const ConnectTimeout = 60 * time.Second

// Tool is one tool a server offers.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	ReadOnly    bool            `json:"readOnly"`
}

// Status is a server as the UI shows it.
type Status struct {
	ID       string    `json:"id"`
	State    State     `json:"state"`
	Protocol string    `json:"protocol"`
	Tools    []Tool    `json:"tools"`
	Error    string    `json:"error,omitempty"`
	Started  time.Time `json:"started"`
}

// Launch is a server's configuration with its secrets resolved: variables
// for a stdio server, headers for an HTTP one.
type Launch struct {
	Config  config.MCPServer
	Env     map[string]string
	Headers map[string]string
	// OAuth signs an HTTP server's requests (MCP-006).
	OAuth auth.OAuthHandler
	// Timeout replaces ConnectTimeout, for a start that waits on the user.
	Timeout time.Duration
}

// ErrSignIn is what an OAuth handler returns when a server needs the user
// to sign in and nobody asked for it now.
var ErrSignIn = errors.New("sign in to use this server")

type server struct {
	// asker is the call in flight on the server just now, which an
	// older server's elicitation/create goes to (MCP-008); with calls
	// from several sessions at once, the latest.
	asker       *Asker
	fingerprint string
	status      Status
	session     *sdk.ClientSession
	proc        environment.Process
	cancel      context.CancelFunc
}

// Manager is safe for concurrent use.
type Manager struct {
	spawner environment.Spawner

	mu      sync.Mutex
	servers map[string]*server
}

// New returns a manager that starts stdio servers through spawner.
func New(spawner environment.Spawner) *Manager {
	return &Manager{spawner: spawner, servers: map[string]*server{}}
}

// Start starts the server, or does nothing when it already runs with the
// same configuration; a changed configuration restarts it. It returns once
// the server's tools are listed or starting failed.
func (m *Manager) Start(ctx context.Context, l Launch) error {
	id, fp := l.Config.ID, l.Config.Fingerprint()+fmt.Sprint(len(l.Env), len(l.Headers))
	m.mu.Lock()
	if s, ok := m.servers[id]; ok && s.fingerprint == fp && (s.status.State == Running || s.status.State == Starting) {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()
	m.Stop(id)

	runCtx, cancel := context.WithCancel(context.Background())
	s := &server{fingerprint: fp, cancel: cancel, status: Status{ID: id, State: Starting, Tools: []Tool{}}}
	m.mu.Lock()
	m.servers[id] = s
	m.mu.Unlock()

	fail := func(err error) error {
		cancel()
		msg := err.Error()
		if s.proc != nil {
			if tail := strings.TrimSpace(s.proc.Stderr()); tail != "" {
				msg += ": " + lastLines(tail, 6)
			}
		}
		m.mu.Lock()
		s.status.State, s.status.Error = Failed, msg
		if errors.Is(err, ErrSignIn) || strings.Contains(msg, ErrSignIn.Error()) {
			s.status.State, s.status.Error = SignIn, ErrSignIn.Error()
		}
		m.mu.Unlock()
		return errors.New(msg)
	}

	var transport sdk.Transport
	switch l.Config.Transport {
	case "http":
		transport = &sdk.StreamableClientTransport{
			Endpoint:             l.Config.URL,
			HTTPClient:           &http.Client{Transport: headers{set: merge(l.Config.Headers, l.Headers), host: hostOf(l.Config.URL), scheme: schemeOf(l.Config.URL), next: http.DefaultTransport}},
			DisableStandaloneSSE: true,
			MaxRetries:           2,
			OAuthHandler:         l.OAuth,
		}
	default:
		if m.spawner == nil {
			return fail(errors.New("this environment cannot start processes"))
		}
		p, err := m.spawner.Spawn(runCtx, environment.SpawnSpec{
			Argv: append([]string{l.Config.Command}, l.Config.Args...), Env: merge(l.Config.Env, l.Env), LoginShell: true,
		})
		if err != nil {
			return fail(err)
		}
		s.proc = p
		transport = &sdk.IOTransport{Reader: p.Stdout(), Writer: p.Stdin()}
	}

	timeout := ConnectTimeout
	if l.Timeout > 0 {
		timeout = l.Timeout
	}
	cctx, ccancel := context.WithTimeout(ctx, timeout)
	defer ccancel()
	// Stop ends a start in progress too, a sign-in waiting on the browser
	// included.
	defer context.AfterFunc(runCtx, ccancel)()
	client := sdk.NewClient(&sdk.Implementation{Name: "freeloop", Version: "0.1"}, &sdk.ClientOptions{ElicitationHandler: m.elicit(id)})
	cs, err := client.Connect(cctx, transport, nil)
	if err != nil {
		return fail(fmt.Errorf("connect: %w", err))
	}
	tools, err := listTools(cctx, cs)
	if err != nil {
		cs.Close()
		return fail(fmt.Errorf("list tools: %w", err))
	}
	m.mu.Lock()
	if m.servers[id] != s || runCtx.Err() != nil {
		// Stopped or replaced while it started.
		m.mu.Unlock()
		cs.Close()
		return errors.New("the server was stopped while it started")
	}
	s.session = cs
	s.status.State, s.status.Tools, s.status.Started, s.status.Error = Running, tools, time.Now().UTC(), ""
	if ir := cs.InitializeResult(); ir != nil {
		s.status.Protocol = ir.ProtocolVersion
	}
	m.mu.Unlock()

	go func() {
		cs.Wait()
		m.mu.Lock()
		defer m.mu.Unlock()
		if s.status.State == Running {
			s.status.State, s.status.Error = Failed, "the server stopped"
			if s.proc != nil {
				if tail := strings.TrimSpace(s.proc.Stderr()); tail != "" {
					s.status.Error += ": " + lastLines(tail, 6)
				}
			}
		}
	}()
	return nil
}

func listTools(ctx context.Context, cs *sdk.ClientSession) ([]Tool, error) {
	out := []Tool{}
	params := &sdk.ListToolsParams{}
	for range 20 {
		res, err := cs.ListTools(ctx, params)
		if err != nil {
			return nil, err
		}
		for _, t := range res.Tools {
			schema, _ := json.Marshal(t.InputSchema)
			tool := Tool{Name: t.Name, Description: t.Description, InputSchema: schema}
			if t.Annotations != nil {
				tool.ReadOnly = t.Annotations.ReadOnlyHint
			}
			out = append(out, tool)
		}
		if res.NextCursor == "" {
			break
		}
		params = &sdk.ListToolsParams{Cursor: res.NextCursor}
	}
	slices.SortFunc(out, func(a, b Tool) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// Stop ends a server.
func (m *Manager) Stop(id string) {
	m.mu.Lock()
	s, ok := m.servers[id]
	delete(m.servers, id)
	m.mu.Unlock()
	if !ok {
		return
	}
	if s.session != nil {
		s.session.Close()
	}
	s.cancel()
	if s.proc != nil {
		s.proc.Kill()
	}
}

// StopAll ends every server.
func (m *Manager) StopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.servers))
	for id := range m.servers {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Stop(id)
	}
}

// Status returns a server's state; a server never started is stopped.
func (m *Manager) Status(id string) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.servers[id]; ok {
		st := s.status
		st.Tools = slices.Clone(st.Tools)
		return st
	}
	return Status{ID: id, State: Stopped, Tools: []Tool{}}
}

// Result is a tool call's result as text for the model.
type Result struct {
	Text    string
	IsError bool
}

// Call runs a tool on a running server.
func (m *Manager) Call(ctx context.Context, id, tool string, args json.RawMessage) (Result, error) {
	m.mu.Lock()
	s, ok := m.servers[id]
	var cs *sdk.ClientSession
	if ok && s.status.State == Running {
		cs = s.session
	}
	m.mu.Unlock()
	if cs == nil {
		return Result{}, fmt.Errorf("the MCP server %s is not running", id)
	}
	var arguments any = map[string]any{}
	if len(args) > 0 && string(args) != "null" {
		arguments = args
	}
	if a := askerFrom(ctx); a != nil {
		// The request arrives on the server's own context; answer it on
		// the call's, which names the tool call and ends with it.
		bound := Asker(func(_ context.Context, server string, e Elicitation) (Answer, error) { return a(ctx, server, e) })
		m.mu.Lock()
		s.asker = &bound
		m.mu.Unlock()
		defer func() {
			m.mu.Lock()
			if s.asker == &bound {
				s.asker = nil
			}
			m.mu.Unlock()
		}()
	}
	res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: tool, Arguments: arguments})
	if err != nil {
		return Result{}, err
	}
	return Result{Text: render(res), IsError: res.IsError}, nil
}

// render turns a tool result's content into text; media become a note.
func render(res *sdk.CallToolResult) string {
	var parts []string
	for _, c := range res.Content {
		switch v := c.(type) {
		case *sdk.TextContent:
			parts = append(parts, v.Text)
		case *sdk.ImageContent:
			parts = append(parts, fmt.Sprintf("[image, %s, %d bytes]", v.MIMEType, len(v.Data)))
		case *sdk.AudioContent:
			parts = append(parts, fmt.Sprintf("[audio, %s, %d bytes]", v.MIMEType, len(v.Data)))
		case *sdk.EmbeddedResource:
			if v.Resource != nil && v.Resource.Text != "" {
				parts = append(parts, v.Resource.Text)
			} else if v.Resource != nil {
				parts = append(parts, "[resource "+v.Resource.URI+"]")
			}
		case *sdk.ResourceLink:
			parts = append(parts, "[resource link "+v.URI+"]")
		default:
			b, _ := json.Marshal(c)
			parts = append(parts, string(b))
		}
	}
	if len(parts) == 0 && res.StructuredContent != nil {
		b, _ := json.Marshal(res.StructuredContent)
		parts = append(parts, string(b))
	}
	return strings.Join(parts, "\n")
}

func merge(a, b map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

// headers adds configured headers, secret ones included, to every request.
type headers struct {
	set map[string]string
	// host is the server's; a redirect elsewhere gets none of the headers,
	// which can hold secrets, and neither does a redirect from https to http.
	host   string
	scheme string
	next   http.RoundTripper
}

func (h headers) RoundTrip(r *http.Request) (*http.Response, error) {
	if !strings.EqualFold(r.URL.Host, h.host) || (h.scheme == "https" && r.URL.Scheme != "https") {
		return h.next.RoundTrip(r)
	}
	r = r.Clone(r.Context())
	for k, v := range h.set {
		r.Header.Set(k, v)
	}
	return h.next.RoundTrip(r)
}

func schemeOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Scheme
}

func loopback(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return host == "localhost" || strings.HasSuffix(host, ".localhost")
}

// Warning says why a stdio command deserves a second look before it first
// runs (MCP-005): it fetches and runs code from a package registry or an
// image that is not pinned.
func Warning(s config.MCPServer) string {
	if s.Transport != "stdio" {
		if u, err := url.Parse(s.URL); err == nil && u.Scheme == "http" && !loopback(u.Hostname()) {
			return "this address is plain http, so what is sent to it, secrets included, is not encrypted."
		}
		return ""
	}
	cmd := s.Command
	if i := strings.LastIndex(cmd, "/"); i >= 0 {
		cmd = cmd[i+1:]
	}
	args := strings.Join(s.Args, " ")
	switch cmd {
	case "npx", "bunx", "pnpx":
		return "npx downloads and runs the latest version of an npm package each time; pin a version (package@1.2.3) where you can."
	case "uvx", "pipx":
		return cmd + " downloads and runs a Python package; pin a version where you can."
	case "docker", "podman":
		if strings.Contains(args, "run") {
			for _, a := range s.Args {
				if !strings.HasPrefix(a, "-") && strings.Contains(a, "/") && !strings.Contains(a, "@sha256:") && !strings.Contains(a, ":") {
					return "the image " + a + " has no tag or digest; it can change under you."
				}
			}
			return "docker run starts a container image; check that it is pinned to a digest."
		}
	}
	return ""
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}
