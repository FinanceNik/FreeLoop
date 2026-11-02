package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/mcp"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// MCP servers (MCP-001, MCP-003, MCP-005) are configured in mcp.yaml and run
// app-wide: a server starts when a session that uses it starts, or from the
// MCP page, and only once the user has approved its exact configuration.

// sessionServerWait bounds how long a session's first prompt waits for its
// servers; a slower one adds its tools at a later model call.
const sessionServerWait = 15 * time.Second

// MCPServerInfo is a server as the MCP page shows it: secret names, never
// values.
type MCPServerInfo struct {
	config.MCPServer
	SecretNames []string   `json:"secretNames"`
	Status      mcp.Status `json:"status"`
	// NeedsApproval is set until the user confirms this configuration;
	// Fingerprint is what they confirm, and Warning a reason to look twice.
	NeedsApproval bool   `json:"needsApproval"`
	Fingerprint   string `json:"fingerprint"`
	Warning       string `json:"warning,omitempty"`
	// SignedIn says the Keychain holds a sign-in for the server; SignInURL
	// is the page a sign-in in progress waits on (MCP-006).
	SignedIn  bool   `json:"signedIn"`
	SignInURL string `json:"signInUrl,omitempty"`
}

func (e *Engine) loadMCP() {
	list, issues, err := config.LoadMCP(e.dataDir)
	if err != nil {
		issues = append(issues, config.Issue{File: config.MCPFileName, Message: err.Error()})
	}
	e.mu.Lock()
	old := e.mcpServers
	e.mcpServers = list
	e.issues[issueMCP] = issues
	e.mu.Unlock()
	// A server whose configuration changed or went away stops; the next
	// session or start uses the new one.
	for _, o := range old {
		i := slices.IndexFunc(list, func(s config.MCPServer) bool { return s.ID == o.ID })
		if i < 0 || list[i].Fingerprint() != o.Fingerprint() || list[i].Approved != o.Approved {
			e.mcp.Stop(o.ID)
		}
	}
}

func (e *Engine) mcpServer(id string) (config.MCPServer, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.mcpServers {
		if s.ID == id {
			return s, nil
		}
	}
	return config.MCPServer{}, fmt.Errorf("no MCP server %q", id)
}

func (e *Engine) mcpInfo(s config.MCPServer) MCPServerInfo {
	if s.Env == nil {
		s.Env = map[string]string{}
	}
	if s.Headers == nil {
		s.Headers = map[string]string{}
	}
	if s.Args == nil {
		s.Args = []string{}
	}
	info := MCPServerInfo{
		MCPServer: s, SecretNames: slices.Sorted(maps.Keys(s.Secrets)), Status: e.mcp.Status(s.ID),
		NeedsApproval: !e.mcpApproved(s), Fingerprint: s.Fingerprint(), Warning: mcp.Warning(s),
		SignInURL: e.signInURL(s.ID),
	}
	if s.Transport == "http" {
		_, info.SignedIn = e.loadOAuth(s)
	}
	return info
}

// ListMCPServers returns every configured server with its state.
func (e *Engine) ListMCPServers() []MCPServerInfo {
	e.mu.Lock()
	list := slices.Clone(e.mcpServers)
	e.mu.Unlock()
	out := make([]MCPServerInfo, 0, len(list))
	for _, s := range list {
		out = append(out, e.mcpInfo(s))
	}
	return out
}

// SaveMCPServerRequest adds or changes a server. Secrets holds new values,
// stored in the Keychain; secrets not named stay as they are.
type SaveMCPServerRequest struct {
	Server        config.MCPServer  `json:"server"`
	Secrets       map[string]string `json:"secrets"`
	RemoveSecrets []string          `json:"removeSecrets"`
	// imported marks a server from another program's settings.
	imported bool
}

func mcpSecretRef(id, name string) secrets.Ref {
	return secrets.Ref{Service: "freeloop", Account: "mcp/" + id + "/" + name}
}

// sameDestination reports whether two configurations of a server send what they
// are given to the same place.
func sameDestination(a, b config.MCPServer) bool {
	return a.Transport == b.Transport && a.URL == b.URL && a.Command == b.Command
}

// SaveMCPServer writes the server into mcp.yaml. Any change to what it runs
// or where it connects needs a new approval.
func (e *Engine) SaveMCPServer(req SaveMCPServerRequest) (MCPServerInfo, error) {
	s := req.Server
	if s.Transport == "" {
		s.Transport = "stdio"
	}
	e.mu.Lock()
	list := slices.Clone(e.mcpServers)
	e.mu.Unlock()
	i := slices.IndexFunc(list, func(x config.MCPServer) bool { return x.ID == s.ID })
	s.Secrets = map[string]string{}
	if i >= 0 {
		s.Approved, s.Source = list[i].Approved, list[i].Source
		// A secret is for the address or command it was given for. Sent on to a
		// new one, a repository's .mcp.json could collect a stored token, so an
		// import inherits none unless nothing changed. The user changing a
		// command's arguments here is still talking to the same program.
		same := sameDestination(list[i], s)
		if req.imported {
			same = same && slices.Equal(list[i].Args, s.Args)
		}
		if same {
			maps.Copy(s.Secrets, list[i].Secrets)
		} else {
			for name := range list[i].Secrets {
				e.secrets.Delete(mcpSecretRef(s.ID, name))
			}
		}
	}
	for _, name := range req.RemoveSecrets {
		delete(s.Secrets, name)
		e.secrets.Delete(mcpSecretRef(s.ID, name))
	}
	for name, value := range req.Secrets {
		if name == "" || value == "" {
			continue
		}
		ref := mcpSecretRef(s.ID, name)
		if err := e.secrets.Set(ref, value); err != nil {
			return MCPServerInfo{}, err
		}
		e.remember(value)
		s.Secrets[name] = ref.String()
	}
	if err := s.Validate(); err != nil {
		return MCPServerInfo{}, err
	}
	if i >= 0 {
		list[i] = s
	} else {
		list = append(list, s)
	}
	if err := config.SaveMCP(e.dataDir, list); err != nil {
		return MCPServerInfo{}, err
	}
	e.loadMCP()
	return e.mcpInfo(s), nil
}

// DeleteMCPServer stops a server and removes it and its secrets.
func (e *Engine) DeleteMCPServer(id string) error {
	s, err := e.mcpServer(id)
	if err != nil {
		return err
	}
	e.mcp.Stop(id)
	for name := range s.Secrets {
		e.secrets.Delete(mcpSecretRef(id, name))
	}
	e.secrets.Delete(oauthRef(id))
	e.mu.Lock()
	list := slices.DeleteFunc(slices.Clone(e.mcpServers), func(x config.MCPServer) bool { return x.ID == id })
	e.mu.Unlock()
	if err := config.SaveMCP(e.dataDir, list); err != nil {
		return err
	}
	e.loadMCP()
	return nil
}

// ApproveMCPServer records the user's confirmation of the configuration
// with fingerprint (MCP-005), which must still be the current one, and
// starts the server.
func (e *Engine) ApproveMCPServer(ctx context.Context, id, fingerprint string) (MCPServerInfo, error) {
	s, err := e.mcpServer(id)
	if err != nil {
		return MCPServerInfo{}, err
	}
	if s.Fingerprint() != fingerprint {
		return MCPServerInfo{}, errors.New("the server's configuration changed since you looked at it; review it again")
	}
	mark, err := e.approvalMark(fingerprint)
	if err != nil {
		return MCPServerInfo{}, err
	}
	e.mu.Lock()
	list := slices.Clone(e.mcpServers)
	e.mu.Unlock()
	for i := range list {
		if list[i].ID == id {
			list[i].Approved = mark
		}
	}
	if err := config.SaveMCP(e.dataDir, list); err != nil {
		return MCPServerInfo{}, err
	}
	e.loadMCP()
	err = e.StartMCPServer(ctx, id)
	s, _ = e.mcpServer(id)
	return e.mcpInfo(s), err
}

// StartMCPServer starts an approved server (or restarts it).
func (e *Engine) StartMCPServer(ctx context.Context, id string) error {
	s, err := e.mcpServer(id)
	if err != nil {
		return err
	}
	if !e.mcpApproved(s) {
		return fmt.Errorf("approve %s before it runs", id)
	}
	l, err := e.mcpLaunch(s)
	if err != nil {
		return err
	}
	if isSigningIn(ctx) {
		l.Timeout = signInTimeout + time.Minute
	}
	err = e.mcp.Start(ctx, l)
	// Waiting for the user's sign-in is a state the server shows, not a
	// failure of the start.
	if err != nil && e.mcp.Status(id).State == mcp.SignIn {
		return nil
	}
	return err
}

// StopMCPServer stops a server.
func (e *Engine) StopMCPServer(id string) { e.mcp.Stop(id) }

// mcpLaunch resolves a server's secrets: variables for stdio, headers for
// HTTP. Every value joins the redaction list.
func (e *Engine) mcpLaunch(s config.MCPServer) (mcp.Launch, error) {
	l := mcp.Launch{Config: s, Env: map[string]string{}, Headers: map[string]string{}}
	for name, refStr := range s.Secrets {
		ref, err := ownRef(refStr, mcpSecretRef(s.ID, name))
		if err != nil {
			return l, fmt.Errorf("%s: secret %s: %w", s.ID, name, err)
		}
		v, err := e.secrets.Get(ref)
		if err != nil {
			return l, fmt.Errorf("%s: secret %s: %w", s.ID, name, err)
		}
		e.remember(v)
		if s.Transport == "http" {
			l.Headers[name] = v
		} else {
			l.Env[name] = v
		}
	}
	if s.Transport == "http" {
		h, err := e.oauthHandler(s)
		if err != nil {
			return l, fmt.Errorf("%s: sign-in: %w", s.ID, err)
		}
		l.OAuth = h
	}
	return l, nil
}

// sessionServers are the approved servers a session uses: those enabled
// for everyone and those its profile adds.
func (e *Engine) sessionServers(prof config.Profile) []config.MCPServer {
	e.mu.Lock()
	list := slices.Clone(e.mcpServers)
	e.mu.Unlock()
	var out []config.MCPServer
	for _, s := range list {
		if (s.Enabled || slices.Contains(prof.MCPServers, s.ID)) && e.mcpApproved(s) {
			out = append(out, s)
		}
	}
	return out
}

// startSessionServers starts a session's servers that are not running,
// waiting at most sessionServerWait.
func (e *Engine) startSessionServers(prof config.Profile) {
	var wg sync.WaitGroup
	for _, s := range e.sessionServers(prof) {
		if st := e.mcp.Status(s.ID).State; st == mcp.Running || st == mcp.Starting {
			continue
		}
		wg.Add(1)
		e.bg.Go(func() {
			defer wg.Done()
			if err := e.StartMCPServer(context.Background(), s.ID); err != nil {
				log.Printf("engine: MCP server %s: %s", s.ID, e.redactText(err.Error()))
			}
		})
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(sessionServerWait):
	}
}

var unsafeToolChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// mcpToolName is mcp__<server>__<tool>, made safe for providers that allow
// only letters, digits, _ and - in at most 64 characters.
func mcpToolName(server, tool string) string {
	name := "mcp__" + server + "__" + unsafeToolChars.ReplaceAllString(tool, "_")
	if len(name) > 64 {
		sum := sha256.Sum256([]byte(name))
		name = name[:55] + "_" + hex.EncodeToString(sum[:])[:8]
	}
	return name
}

// sessionMCP is a session's harness.MCPSource. It reads the profile and
// the servers' state at every call.
type sessionMCP struct {
	e         *Engine
	sid       string
	projectID string
	root      string
	profileID string
}

func (s sessionMCP) profile() config.Profile {
	project, err := s.e.store.GetProject(context.Background(), s.projectID)
	if err != nil {
		project = session.Project{ID: s.projectID, Root: s.root}
	}
	prof, err := s.e.profileFor(project, s.profileID)
	if err != nil {
		return config.BuiltinDefaultProfile()
	}
	return prof
}

func (s sessionMCP) Tools() []harness.MCPTool {
	var out []harness.MCPTool
	for _, srv := range s.e.sessionServers(s.profile()) {
		st := s.e.mcp.Status(srv.ID)
		if st.State != mcp.Running {
			continue
		}
		for _, t := range st.Tools {
			schema := map[string]any{}
			json.Unmarshal(t.InputSchema, &schema)
			if len(schema) == 0 {
				schema = map[string]any{"type": "object"}
			}
			desc := t.Description
			if desc == "" {
				desc = "Tool " + t.Name + " of the MCP server " + srv.ID + "."
			}
			out = append(out, harness.MCPTool{Name: mcpToolName(srv.ID, t.Name), Description: desc, InputSchema: schema, Server: srv.ID, Tool: t.Name})
		}
	}
	return out
}

func (s sessionMCP) Call(ctx context.Context, name string, input json.RawMessage) (string, bool, error) {
	for _, t := range s.Tools() {
		if t.Name == name {
			res, err := s.e.mcp.Call(mcp.WithAsker(ctx, s.ask), t.Server, t.Tool, input)
			return res.Text, res.IsError, err
		}
	}
	return "", false, fmt.Errorf("the MCP tool %s is not available", strings.TrimSpace(name))
}

// ask puts an MCP server's request for input to the user (MCP-008): a form
// on the tool call that caused it, always asked, whatever the mode or
// rules; Stop cancels it like any request.
func (s sessionMCP) ask(ctx context.Context, server string, req mcp.Elicitation) (mcp.Answer, error) {
	title := server + " asks: " + req.Message
	if len(title) > 200 {
		title = title[:200] + "…"
	}
	d := s.e.broker.Request(ctx, permission.Context{Root: s.root}, permission.Request{
		SessionID: s.sid, ToolCallID: harness.ToolCall(ctx), Tool: mcpToolName(server, "input"), Kind: permission.KindOther,
		Title: title, Options: permission.FormOptions,
		Form: &permission.Form{Server: server, Message: req.Message, Schema: req.Schema, URL: req.URL},
	})
	switch {
	case d.Outcome == permission.Cancelled:
		return mcp.Answer{Action: "cancel"}, nil
	case d.Outcome == permission.Allowed:
		return mcp.Answer{Action: "accept", Content: d.Answers}, nil
	}
	return mcp.Answer{Action: "decline"}, nil
}
