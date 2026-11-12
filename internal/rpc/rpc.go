// Package rpc is the headless API (EXT-004): JSON-RPC 2.0 over a Unix
// socket in the app's data folder, one message per line, as a thin adapter
// over the engine like the Wails service. The app serves it while it runs;
// the freeloop command line is a client. Everything it does goes through
// the engine, so the GUI shows it live and the permission broker judges
// every tool call as usual.
package rpc

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/event"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error is a JSON-RPC error.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

// Server serves the API on a socket.
type Server struct {
	e  *engine.Engine
	ln net.Listener
	wg sync.WaitGroup
	// ctx ends with Close, and with it a call that waits (sessions.wait);
	// conns are the open connections, which Close ends too.
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	conns  map[net.Conn]struct{}
}

// Listen opens the socket at path. Another running app on the same data
// folder makes it fail; a socket a crashed run left behind is cleared.
func Listen(e *engine.Engine, path string) (*Server, error) {
	if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
		c.Close()
		return nil, errors.New("another Freeloop is serving this data folder")
	}
	if err := config.ClearSocket(path); err != nil {
		return nil, err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{e: e, ln: ln, ctx: ctx, cancel: cancel, conns: map[net.Conn]struct{}{}}, nil
}

// Serve accepts connections until Close.
func (s *Server) Serve() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		if s.ctx.Err() != nil {
			s.mu.Unlock()
			c.Close()
			return
		}
		s.conns[c] = struct{}{}
		s.mu.Unlock()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() {
				s.mu.Lock()
				delete(s.conns, c)
				s.mu.Unlock()
			}()
			s.conn(c)
		}()
	}
}

// Close stops serving, ends the open connections and the calls waiting on
// them, and removes the socket.
func (s *Server) Close() error {
	err := s.ln.Close()
	s.mu.Lock()
	s.cancel()
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	return err
}

// conn answers one client's requests, one at a time, in order.
func (s *Server) conn(c net.Conn) {
	defer c.Close()
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	enc := json.NewEncoder(c)
	for sc.Scan() {
		var req request
		resp := response{JSONRPC: "2.0"}
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			resp.Error = &Error{Code: -32700, Message: "parse error: " + err.Error()}
		} else {
			resp.ID = req.ID
			res, err := s.call(s.ctx, req.Method, req.Params)
			if err != nil {
				var re *Error
				if !errors.As(err, &re) {
					re = &Error{Code: -32000, Message: err.Error()}
				}
				resp.Error = re
			} else {
				resp.Result = res
			}
		}
		if err := enc.Encode(resp); err != nil {
			log.Printf("rpc: %v", err)
			return
		}
	}
}

// Methods.
type (
	idParams   struct{ ID string }
	promptP    struct{ ID, Text string }
	optimizerP struct{ ID, Optimizer string }
	rewriteP   struct{ ID, Command string }
	eventsP    struct {
		ID    string
		After int64
		// Limit is the most events to return; 0 or more than a page asks for a page.
		Limit int
	}
	waitP struct {
		ID        string
		After     int64
		TimeoutMs int
	}
	decideP struct {
		RequestID string
		OptionID  string
		Reason    string
		// Answers fill in a form request (MCP-008).
		Answers map[string]any
	}
)

func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return &Error{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	return nil
}

func (s *Server) call(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	e := s.e
	switch method {
	case "sessions.list":
		return e.ListSessions(ctx)
	case "sessions.get":
		var p idParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return e.GetSession(ctx, p.ID)
	case "sessions.create":
		var p engine.CreateSessionRequest
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		// What the new-session form preselects (SET-004).
		if d := e.Config().Defaults; p.ProviderID == "" && p.ProfileID == "" {
			p.ProviderID, p.Model = d.ProviderID, cmp.Or(p.Model, d.Model)
		}
		return e.CreateSession(ctx, p)
	case "sessions.prompt":
		var p promptP
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return e.Prompt(ctx, p.ID, p.Text)
	case "sessions.cancel":
		var p idParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return struct{}{}, e.Cancel(p.ID)
	case "sessions.events":
		var p eventsP
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		if p.Limit <= 0 || p.Limit > engine.MaxEventsPage {
			p.Limit = engine.MaxEventsPage
		}
		return e.Events(ctx, p.ID, p.After, p.Limit)
	case "sessions.wait":
		var p waitP
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return s.wait(ctx, p)
	case "sessions.optimizers":
		var p idParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return e.Optimizers(ctx, p.ID)
	case "sessions.setOptimizer":
		var p optimizerP
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return struct{}{}, e.SetOptimizer(ctx, p.ID, p.Optimizer)
	case "sessions.options":
		var p idParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return e.SessionOptions(ctx, p.ID)
	case "sessions.setOption":
		var p struct {
			ID     string `json:"id"`
			Option string `json:"option"`
			Value  string `json:"value"`
		}
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return struct{}{}, e.SetSessionOption(ctx, p.ID, p.Option, p.Value)
	case "sessions.importable":
		var p struct {
			Profile string `json:"profile"`
			Folder  string `json:"folder"`
		}
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return e.AgentSessions(ctx, p.Profile, p.Folder)
	case "sessions.import":
		var p engine.ImportSessionRequest
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return e.ImportAgentSession(ctx, p)
	case "sessions.rewriteForAgent":
		var p rewriteP
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		made, err := e.RewriteForAgent(ctx, p.ID, p.Command)
		return map[string]string{"command": made}, err
	case "policy.check", "policy.overview":
		// What the broker would do with a call, and what decides it (SEC-018).
		var p engine.PolicyQuery
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		if method == "policy.check" {
			return e.CheckPolicy(ctx, p)
		}
		return e.PolicyOverview(ctx, p)
	case "approvals.list":
		return e.PendingPermissions(), nil
	case "approvals.decide":
		var p decideP
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return struct{}{}, e.Decide(engine.DecideRequest{RequestID: p.RequestID, OptionID: p.OptionID, Reason: p.Reason, Answers: p.Answers})
	}
	return nil, &Error{Code: -32601, Message: fmt.Sprintf("no method %q", method)}
}

// wait returns a session's events after p.After once it has new ones, or
// when the timeout (default 30 s, at most 5 min) runs out with none.
func (s *Server) wait(ctx context.Context, p waitP) ([]event.Event, error) {
	timeout := time.Duration(p.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	timeout = min(timeout, 5*time.Minute)
	ch, cancel := s.e.Subscribe()
	defer cancel()
	evs, err := s.e.Events(ctx, p.ID, p.After, engine.MaxEventsPage)
	if err != nil || len(evs) > 0 {
		return evs, err
	}
	deadline := time.After(timeout)
	for {
		select {
		case ev := <-ch:
			if ev.SID == p.ID && ev.Seq > p.After {
				return s.e.Events(ctx, p.ID, p.After, engine.MaxEventsPage)
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			return []event.Event{}, nil
		}
	}
}
