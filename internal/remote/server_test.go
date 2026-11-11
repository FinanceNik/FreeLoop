package remote

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

type phone struct {
	t      *testing.T
	base   string
	client *http.Client
	login  string
	// token is the key the phone holds, sent as a header (never a cookie, which every
	// port of the same host would receive).
	token string
}

// authorize puts the phone's key and login on a request, as the phone app does.
func (p *phone) authorize(req *http.Request) {
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	if p.login != "" {
		req.Header.Set(tsLogin, p.login)
	}
}

func (p *phone) do(method, path string, body any, extra map[string]string) (*http.Response, map[string]any) {
	p.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, p.base+path, r)
	if method == http.MethodPost {
		req.Header.Set("Origin", p.base)
		req.Header.Set(phoneHeader, "1")
		req.Header.Set("Content-Type", "application/json")
	}
	p.authorize(req)
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		p.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func freePort(t *testing.T) int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// started is the server on a real port, with phone access turned on in Settings.
func started(t *testing.T) (*engine.Engine, *Server, *phone) {
	t.Helper()
	dir := t.TempDir()
	e, err := engine.Open(engine.Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir(), Dev: true, WatchInterval: -1, ScheduleInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	cfg := e.Config()
	cfg.Remote = config.Remote{Enabled: true, Port: freePort(t), Host: "laptop.tail1234.ts.net"}
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	s := New(e, dir, fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>phone</title>")}})
	st := e.RemoteState()
	if !st.On {
		t.Fatalf("remote state: %+v", st)
	}
	s.apply(st)
	t.Cleanup(func() { s.apply(engine.RemoteState{}) })
	return e, s, &phone{t: t, base: "http://127.0.0.1:" + strconv.Itoa(st.Port), client: &http.Client{}}
}

func pairPhone(t *testing.T, s *Server, p *phone) {
	t.Helper()
	ps, err := s.StartPairing()
	if err != nil {
		t.Fatal(err)
	}
	resp, out := p.do("POST", "/api/pair", map[string]string{"code": strings.ToLower(ps.Code), "name": "Test phone"}, nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("pair: %d %v", resp.StatusCode, out)
	}
	id := out["id"].(string)
	if _, out := p.do("GET", "/api/pair/"+id, nil, nil); out["answer"] != "waiting" {
		t.Fatalf("before the answer: %v", out)
	}
	if st := s.Status(); st.Pairing == nil || st.Pairing.Phone != "Test phone" {
		t.Fatalf("Settings does not show the waiting phone: %+v", st.Pairing)
	}
	if err := s.AnswerPairing(true); err != nil {
		t.Fatal(err)
	}
	resp, out = p.do("GET", "/api/pair/"+id, nil, nil)
	token, _ := out["token"].(string)
	if out["answer"] != "allowed" || len(token) < 40 {
		t.Fatalf("after allowing: %v", out)
	}
	if c := resp.Header.Get("Set-Cookie"); c != "" {
		t.Errorf("the key was also sent as a cookie: %s", c)
	}
	p.token = token
	if _, out := p.do("GET", "/api/pair/"+id, nil, nil); out["answer"] != "refused" || out["token"] != nil {
		t.Errorf("the key went out twice: %v", out)
	}
}

// GUI-052: only a phone paired with a code and the user's yes on this computer gets
// in, from the expected host, not over Funnel, and not with a page elsewhere's post.
func TestPhoneMustBePairedOnThisComputer(t *testing.T) {
	_, s, p := started(t)
	if resp, _ := p.do("GET", "/api/summary", nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unpaired: %d", resp.StatusCode)
	}
	if resp, _ := p.do("GET", "/phone/", nil, nil); resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatalf("the app's page: %d %v", resp.StatusCode, resp.Header)
	}
	req, _ := http.NewRequest("GET", p.base+"/phone/", nil)
	req.Host = "evil.example"
	if resp, err := p.client.Do(req); err != nil || resp.StatusCode != http.StatusMisdirectedRequest {
		t.Errorf("a rebound name: %v %v", resp.StatusCode, err)
	}
	req, _ = http.NewRequest("GET", p.base+"/phone/", nil)
	req.Host = "laptop.tail1234.ts.net"
	if resp, err := p.client.Do(req); err != nil || resp.StatusCode != http.StatusOK {
		t.Errorf("the tailnet name: %v %v", resp.StatusCode, err)
	}
	if resp, _ := p.do("GET", "/phone/", nil, map[string]string{tsFunnel: "true"}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("over Funnel: %d", resp.StatusCode)
	}

	// No window open, then wrong codes close it.
	if resp, _ := p.do("POST", "/api/pair", map[string]string{"code": "x"}, nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("no window: %d", resp.StatusCode)
	}
	s.StartPairing()
	for i := 0; i < pairTries; i++ {
		p.do("POST", "/api/pair", map[string]string{"code": "AAAA-BBBB-CCCC-DDDD-EEEE"}, nil)
	}
	if st := s.Status(); st.Pairing != nil {
		t.Errorf("the window stayed open after %d wrong codes", pairTries)
	}
	// A post without the app's header, as a form elsewhere sends, is refused.
	ps, _ := s.StartPairing()
	req, _ = http.NewRequest("POST", p.base+"/api/pair", strings.NewReader(`{"code":"`+ps.Code+`"}`))
	req.Header.Set("Origin", "https://evil.example")
	if resp, err := p.client.Do(req); err != nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("a cross-site post: %v %v", resp.StatusCode, err)
	}
	// Refused on this computer: no device.
	_, out := p.do("POST", "/api/pair", map[string]string{"code": ps.Code}, nil)
	s.AnswerPairing(false)
	if _, out := p.do("GET", "/api/pair/"+out["id"].(string), nil, nil); out["answer"] != "refused" {
		t.Errorf("refused: %v", out)
	}
	if len(s.Status().Devices) != 0 {
		t.Fatal("a refused phone was paired")
	}

	p.login = "alice@example.com"
	pairPhone(t, s, p)
	if resp, out := p.do("GET", "/api/summary", nil, nil); resp.StatusCode != http.StatusOK || out["sessions"] == nil {
		t.Fatalf("paired: %d %v", resp.StatusCode, out)
	}
	if st := s.Status(); len(st.Devices) != 1 || st.Devices[0].Login != "alice@example.com" {
		t.Fatalf("devices: %+v", st.Devices)
	}
	// The key under another Tailscale login is refused, and so is the key with no login at
	// all: replayed straight to the port by something that got hold of it.
	p.login = "mallory@example.com"
	if resp, _ := p.do("GET", "/api/summary", nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("another login: %d", resp.StatusCode)
	}
	p.login = ""
	if resp, _ := p.do("GET", "/api/summary", nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no login header: %d", resp.StatusCode)
	}
	// A cookie is not a key: another page on the same host receives one.
	req, _ = http.NewRequest("GET", p.base+"/api/summary", nil)
	req.AddCookie(&http.Cookie{Name: "freeloop_device", Value: p.token})
	req.Header.Set(tsLogin, "alice@example.com")
	if resp, err := p.client.Do(req); err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the key as a cookie: %v %v", resp.StatusCode, err)
	}
	// A key goes stale after 90 days without use.
	s.now = func() time.Time { return time.Now().Add(91 * 24 * time.Hour) }
	p.login = "alice@example.com"
	if resp, _ := p.do("GET", "/api/summary", nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a key unused for 91 days: %d", resp.StatusCode)
	}
	s.now = time.Now
	p.login = "alice@example.com"
	if err := s.RemoveDevice(s.Status().Devices[0].ID); err != nil {
		t.Fatal(err)
	}
	if resp, _ := p.do("GET", "/api/summary", nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a removed phone: %d", resp.StatusCode)
	}
}

func waitEvent(t *testing.T, ch <-chan event.Event, ok func(event.Event) bool) event.Event {
	t.Helper()
	deadline := time.After(20 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ok(ev) {
				return ev
			}
		case <-deadline:
			t.Fatal("timed out")
		}
	}
}

// GUI-052: from the phone a plain request is allowed once and never always, the
// session is followed live and sent a message, and the transcript says each came
// from the phone.
func TestPhoneApprovesOnceAndPrompts(t *testing.T) {
	e, s, p := started(t)
	pairPhone(t, s, p)
	ctx := context.Background()
	ch, cancel := e.Subscribe()
	defer cancel()
	ses, err := e.StartDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ev := waitEvent(t, ch, func(ev event.Event) bool { return ev.SID == ses.ID && ev.Type == event.PermissionRequested })
	var req permission.Request
	json.Unmarshal(ev.Data, &req)

	// The stream sends the session from its start.
	sreq, _ := http.NewRequest("GET", p.base+"/api/sessions/"+ses.ID+"/stream?after=0", nil)
	p.authorize(sreq)
	sresp, err := p.client.Do(sreq)
	if err != nil || sresp.StatusCode != http.StatusOK {
		t.Fatalf("stream: %v %v", sresp.StatusCode, err)
	}
	defer sresp.Body.Close()
	lines := bufio.NewScanner(sresp.Body)
	sawRequest := false
	for !sawRequest && lines.Scan() {
		sawRequest = strings.HasPrefix(lines.Text(), "data:") && strings.Contains(lines.Text(), `"permission_requested"`)
	}
	if !sawRequest {
		t.Fatal("the stream did not send the waiting request")
	}

	_, sum := p.do("GET", "/api/summary", nil, nil)
	if att, _ := sum["attention"].([]any); len(att) == 0 {
		t.Fatalf("the summary does not list the waiting request: %v", sum)
	}
	always := ""
	for _, o := range req.Options {
		if o.Kind == "allow_always" {
			always = o.ID
		}
	}
	if always != "" {
		if resp, _ := p.do("POST", "/api/decide", map[string]string{"requestId": req.ID, "optionId": always}, nil); resp.StatusCode != http.StatusForbidden {
			t.Errorf("allow always from the phone: %d", resp.StatusCode)
		}
	}
	if resp, out := p.do("POST", "/api/decide", map[string]string{"requestId": req.ID, "optionId": "allow_once"}, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("allow once: %d %v", resp.StatusCode, out)
	}
	ev = waitEvent(t, ch, func(ev event.Event) bool { return ev.SID == ses.ID && ev.Type == event.PermissionDecided })
	var d permission.Decision
	json.Unmarshal(ev.Data, &d)
	if d.Via != "phone:Test phone" || d.Actor != permission.ActorUser || d.Outcome != permission.Allowed {
		t.Errorf("decision: %+v", d)
	}
	// Answer the rest on the computer, then message the session from the phone.
	for {
		ev := waitEvent(t, ch, func(ev event.Event) bool {
			return ev.SID == ses.ID && (ev.Type == event.PermissionRequested || ev.Type == event.TurnEnded)
		})
		if ev.Type == event.TurnEnded {
			break
		}
		json.Unmarshal(ev.Data, &req)
		e.Decide(engine.DecideRequest{RequestID: req.ID, OptionID: "allow_once"})
	}
	if resp, out := p.do("POST", "/api/sessions/"+ses.ID+"/prompt", map[string]string{"text": "thanks, from the gym"}, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("prompt: %d %v", resp.StatusCode, out)
	}
	ev = waitEvent(t, ch, func(ev event.Event) bool {
		return ev.SID == ses.ID && ev.Type == event.UserMessageChunk && strings.Contains(string(ev.Data), "from the gym")
	})
	if !strings.Contains(string(ev.Data), `"via":"phone:Test phone"`) {
		t.Errorf("the message does not say it came from the phone: %s", ev.Data)
	}
}

// Off when turned off, and in local-only mode, which would otherwise let
// transcripts leave this computer.
func TestPhoneServerFollowsTheSettings(t *testing.T) {
	e, s, p := started(t)
	cfg := e.Config()
	cfg.Network.LocalOnly = true
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	st := e.RemoteState()
	if st.On || st.Why != "local-only" {
		t.Fatalf("local-only: %+v", st)
	}
	s.apply(st)
	if _, err := p.client.Get(p.base + "/phone/"); err == nil {
		t.Error("still listening in local-only mode")
	}
}

// A phone signed out from the computer loses the streams it has open at once, not at the
// next reconnect.
func TestARemovedPhonesStreamEnds(t *testing.T) {
	_, s, p := started(t)
	pairPhone(t, s, p)
	req, _ := http.NewRequest("GET", p.base+"/api/stream", nil)
	p.authorize(req)
	resp, err := p.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("stream: %v %v", resp.StatusCode, err)
	}
	defer resp.Body.Close()
	lines := bufio.NewScanner(resp.Body)
	for lines.Scan() {
		if strings.HasPrefix(lines.Text(), "data:") {
			break
		}
	}
	if err := s.RemoveDevice(s.Status().Devices[0].ID); err != nil {
		t.Fatal(err)
	}
	ended := make(chan struct{})
	go func() {
		for lines.Scan() {
		}
		close(ended)
	}()
	select {
	case <-ended:
	case <-time.After(3 * time.Second):
		t.Fatal("the removed phone's stream stayed open")
	}
}
