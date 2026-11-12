// Package remote is the phone server (GUI-052): a small HTTP API and a web app for
// following and steering sessions from the user's phone. It listens on 127.0.0.1
// only; the user makes it reachable over their own private network, such as with
// `tailscale serve`, and only phones they paired, confirming on this computer, get
// in. From there a session can be read live, sent a message, stopped, and a plain
// request allowed or denied once; nothing else of the engine is reachable.
package remote

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

const (
	// A phone's key goes unused for this long before it stops working; use renews it.
	keyLife     = 90 * 24 * time.Hour
	pairLife    = 5 * time.Minute
	pairTries   = 5
	maxBody     = 64 << 10
	phoneHeader = "X-Freeloop-Phone"
	// The headers tailscale serve adds. A process on this computer can send them
	// too, so they are a second check, never the gate.
	tsLogin  = "Tailscale-User-Login"
	tsFunnel = "Tailscale-Funnel-Request"
)

// Server is the phone server. Its zero value is not usable; use New.
type Server struct {
	eng     *engine.Engine
	keys    secrets.Store
	dataDir string
	assets  fs.FS
	now     func() time.Time

	mu      sync.Mutex
	srv     *http.Server
	ln      net.Listener
	state   engine.RemoteState
	listErr string
	pairing *pairing
	devices []config.Device
	// streams cancel what a phone has open, so signing it out ends them at once.
	streams map[string]map[int]context.CancelFunc
	nextRun int
	vapid   *ecdsa.PrivateKey
	// pushClient sends to push services; nil means newPushClient(), tests set their own.
	pushClient *http.Client
}

// pairing is an open pairing window: the code the phone sends, and the phone that
// sent it while it waits for the user's answer on this computer.
type pairing struct {
	code    string
	expires time.Time
	tries   int
	claim   *claim
}

type claim struct {
	id      string
	name    string
	login   string
	answer  string // "", "allowed" or "refused"
	token   string
	device  string
	claimed time.Time
}

// New makes the server for eng, its phone app's files under assets (the built
// frontend's phone/ folder).
func New(eng *engine.Engine, dataDir string, keys secrets.Store, assets fs.FS) *Server {
	s := &Server{eng: eng, keys: keys, dataDir: dataDir, assets: assets, now: time.Now}
	if d, err := config.LoadDevices(dataDir); err == nil {
		s.devices = d
	} else {
		log.Printf("remote: %v", err)
	}
	return s
}

// Run keeps the listener in step with the settings until ctx ends.
func (s *Server) Run(ctx context.Context) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		s.apply(s.eng.RemoteState())
		select {
		case <-ctx.Done():
			s.apply(engine.RemoteState{})
			return
		case <-tick.C:
		}
	}
}

// apply starts, moves or stops the listener to match want.
func (s *Server) apply(want engine.RemoteState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	running := s.ln != nil
	if running && want.On && s.state.Port == want.Port {
		s.state = want
		return
	}
	if running {
		s.srv.Close()
		s.srv, s.ln = nil, nil
	}
	s.state, s.listErr = want, ""
	if !want.On {
		s.pairing = nil
		return
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(want.Port)))
	if err != nil {
		// Another copy of the app, or anything else, has the port.
		s.listErr = err.Error()
		return
	}
	s.ln = ln
	s.srv = &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go s.srv.Serve(ln)
}

// Status is what Settings shows about the phone server.
type Status struct {
	engine.RemoteState
	Listening bool   `json:"listening"`
	Error     string `json:"error,omitempty"`
	// Pairing is the open pairing window, if any.
	Pairing *PairingStatus `json:"pairing,omitempty"`
	Devices []Device       `json:"devices"`
}

// PairingStatus is an open pairing window as Settings shows it.
type PairingStatus struct {
	Code    string    `json:"code"`
	Expires time.Time `json:"expires"`
	// Phone is the phone that sent the code, waiting for the user's answer.
	Phone string `json:"phone,omitempty"`
	Login string `json:"login,omitempty"`
}

// Device is a paired phone as Settings lists it.
type Device struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Login    string    `json:"login,omitempty"`
	Paired   time.Time `json:"paired"`
	LastSeen time.Time `json:"lastSeen"`
	// Notify is whether the phone turned notifications on.
	Notify bool `json:"notify"`
}

// Status says how the phone server is.
func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{RemoteState: s.state, Listening: s.ln != nil, Error: s.listErr, Devices: []Device{}}
	if p := s.openPairing(); p != nil {
		st.Pairing = &PairingStatus{Code: groups(p.code), Expires: p.expires}
		if p.claim != nil && p.claim.answer == "" {
			st.Pairing.Phone, st.Pairing.Login = p.claim.name, p.claim.login
		}
	}
	for _, d := range s.devices {
		st.Devices = append(st.Devices, Device{ID: d.ID, Name: d.Name, Login: d.Login, Paired: d.Paired, LastSeen: d.LastSeen, Notify: d.Push != nil})
	}
	return st
}

// openPairing is the pairing window if it is still open. Callers hold s.mu.
func (s *Server) openPairing() *pairing {
	if s.pairing != nil && s.now().After(s.pairing.expires) {
		s.pairing = nil
	}
	return s.pairing
}

// codeAlphabet leaves out letters read as others: no I, L, O, U.
var codeEncoding = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

// StartPairing opens a pairing window and returns its code: 20 characters, 100
// bits, good for one phone, five minutes and five wrong tries.
func (s *Server) StartPairing() (PairingStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return PairingStatus{}, errors.New("the phone server is not running")
	}
	b := make([]byte, 13)
	rand.Read(b)
	code := codeEncoding.EncodeToString(b)[:20]
	s.pairing = &pairing{code: code, expires: s.now().Add(pairLife)}
	return PairingStatus{Code: groups(code), Expires: s.pairing.expires}, nil
}

// AnswerPairing is the user's answer, on this computer, to the phone waiting in the
// pairing window. Only allowed makes a device.
func (s *Server) AnswerPairing(allow bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.openPairing()
	if p == nil || p.claim == nil || p.claim.answer != "" {
		return errors.New("no phone is waiting to be paired")
	}
	if !allow {
		p.claim.answer = "refused"
		s.pairing = nil
		return nil
	}
	token := make([]byte, 32)
	rand.Read(token)
	d := config.Device{ID: newID(), Name: p.claim.name, TokenHash: hashToken(token), Login: p.claim.login, Paired: s.now().UTC(), LastSeen: s.now().UTC()}
	if err := config.SaveDevices(s.dataDir, append(append([]config.Device{}, s.devices...), d)); err != nil {
		return err
	}
	s.devices = append(s.devices, d)
	p.claim.answer, p.claim.token, p.claim.device = "allowed", base64.RawURLEncoding.EncodeToString(token), d.ID
	return nil
}

// CancelPairing closes the pairing window.
func (s *Server) CancelPairing() {
	s.mu.Lock()
	s.pairing = nil
	s.mu.Unlock()
}

// RemoveDevice forgets a paired phone; its next request is refused. An empty id
// forgets them all.
func (s *Server) RemoveDevice(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := []config.Device{}
	for _, d := range s.devices {
		if id != "" && d.ID != id {
			keep = append(keep, d)
		}
	}
	if err := config.SaveDevices(s.dataDir, keep); err != nil {
		return err
	}
	s.devices = keep
	for did, open := range s.streams {
		if id == "" || did == id {
			for _, cancel := range open {
				cancel()
			}
		}
	}
	return nil
}

// RenameDevice names a paired phone.
func (s *Server) RenameDevice(id, name string) error {
	name = cleanName(name)
	if name == "" {
		return errors.New("give the phone a name")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := append([]config.Device{}, s.devices...)
	for i := range next {
		if next[i].ID == id {
			next[i].Name = name
			if err := config.SaveDevices(s.dataDir, next); err != nil {
				return err
			}
			s.devices = next
			return nil
		}
	}
	return errors.New("no such phone")
}

// groups writes a code in groups of four, for reading out.
func groups(code string) string {
	var parts []string
	for i := 0; i < len(code); i += 4 {
		parts = append(parts, code[i:min(len(code), i+4)])
	}
	return strings.Join(parts, "-")
}

// normalCode reads a typed code: case and separators do not matter.
func normalCode(s string) string {
	s = strings.ToUpper(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hashToken(token []byte) string {
	sum := sha256.Sum256(token)
	return hex.EncodeToString(sum[:])
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "dev_" + hex.EncodeToString(b)
}

// cleanName keeps a phone's name short and printable.
func cleanName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if len([]rune(s)) > 40 {
		s = string([]rune(s)[:40])
	}
	return s
}

// device finds the paired phone a request's key belongs to, and notes it was seen.
// The key is sent as a header the phone app adds, not as a cookie: a cookie goes to
// every port of the same host, to whatever else runs there. A phone paired with a
// Tailscale login must show that login, so the key replayed straight at the port by
// something that took it is refused (SEC-020).
func (s *Server) device(r *http.Request) (config.Device, bool) {
	value, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return config.Device{}, false
	}
	token, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(token) != 32 {
		return config.Device{}, false
	}
	hash := hashToken(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, d := range s.devices {
		if subtle.ConstantTimeCompare([]byte(d.TokenHash), []byte(hash)) != 1 {
			continue
		}
		if d.Login != "" && r.Header.Get(tsLogin) != d.Login {
			return config.Device{}, false
		}
		if s.now().Sub(d.LastSeen) > keyLife {
			return config.Device{}, false
		}
		if s.now().Sub(d.LastSeen) > time.Hour {
			s.devices[i].LastSeen = s.now().UTC()
			config.SaveDevices(s.dataDir, s.devices)
		}
		return s.devices[i], true
	}
	return config.Device{}, false
}

// track registers a cancel for what the phone has open, and returns how to forget it.
// A phone signed out meanwhile gets nothing.
func (s *Server) track(id string, cancel context.CancelFunc) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.ContainsFunc(s.devices, func(d config.Device) bool { return d.ID == id }) {
		cancel()
		return func() {}
	}
	if s.streams == nil {
		s.streams = map[string]map[int]context.CancelFunc{}
	}
	if s.streams[id] == nil {
		s.streams[id] = map[int]context.CancelFunc{}
	}
	s.nextRun++
	n := s.nextRun
	s.streams[id][n] = cancel
	return func() {
		s.mu.Lock()
		delete(s.streams[id], n)
		s.mu.Unlock()
		cancel()
	}
}

// allowedHost checks a request's Host: this computer's address, or the name the
// phone reaches it by. Anything else is a page elsewhere rebinding a name to this
// computer.
func (s *Server) allowedHost(r *http.Request) bool {
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		host, port = r.Host, ""
	}
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	if (host == "127.0.0.1" || host == "localhost") && port == strconv.Itoa(st.Port) {
		return true
	}
	want := strings.ToLower(strings.TrimSuffix(st.Host, "."))
	if h, _, err := net.SplitHostPort(want); err == nil {
		want = h
	}
	return want != "" && strings.EqualFold(strings.TrimSuffix(host, "."), want)
}

// Handler serves the phone app and its API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	files := http.StripPrefix("/phone/", http.FileServer(http.FS(s.assets)))
	// The install files get their types here, not from the system's table, which may
	// lack .webmanifest; the worker may control only /phone/.
	phone := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/phone/manifest.webmanifest":
			w.Header().Set("Content-Type", "application/manifest+json")
		case "/phone/sw.js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			w.Header().Set("Service-Worker-Allowed", "/phone/")
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
	mux.Handle("GET /phone/", phone)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/phone/", http.StatusFound) })
	mux.HandleFunc("POST /api/pair", s.pair)
	mux.HandleFunc("GET /api/pair/{id}", s.pairAnswer)
	mux.HandleFunc("GET /api/summary", s.authed(s.summary))
	mux.HandleFunc("GET /api/stream", s.authed(s.summaryStream))
	mux.HandleFunc("GET /api/sessions/{id}/events", s.authed(s.events))
	mux.HandleFunc("GET /api/sessions/{id}/stream", s.authed(s.sessionStream))
	mux.HandleFunc("POST /api/sessions/{id}/prompt", s.authed(s.prompt))
	mux.HandleFunc("POST /api/sessions/{id}/cancel", s.authed(s.cancel))
	mux.HandleFunc("POST /api/decide", s.authed(s.decide))
	mux.HandleFunc("GET /api/push/key", s.authed(s.pushKey))
	mux.HandleFunc("POST /api/push/subscribe", s.authed(s.pushSubscribe))
	mux.HandleFunc("POST /api/push/unsubscribe", s.authed(s.pushUnsubscribe))
	mux.HandleFunc("POST /api/logout", s.authed(s.logout))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; worker-src 'self'; manifest-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'; object-src 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		switch {
		case r.Header.Get(tsFunnel) != "":
			// Tailscale Funnel would put this on the internet: never.
			http.Error(w, "not over Tailscale Funnel", http.StatusForbidden)
			return
		case !s.allowedHost(r):
			http.Error(w, "unknown host", http.StatusMisdirectedRequest)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
			if r.Method == http.MethodPost && !s.sameOrigin(r) {
				http.Error(w, "not from the phone app", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		}
		mux.ServeHTTP(w, r)
	})
}

// sameOrigin checks that a POST comes from the phone app's own page: its Origin is
// this host, and it carries the app's header, which a form elsewhere cannot.
func (s *Server) sameOrigin(r *http.Request) bool {
	if r.Header.Get(phoneHeader) != "1" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	i := strings.Index(origin, "://")
	return i > 0 && strings.EqualFold(origin[i+3:], r.Host)
}

type handler func(w http.ResponseWriter, r *http.Request, d config.Device)

func (s *Server) authed(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := s.device(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "pair this phone first"})
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer s.track(d.ID, cancel)()
		h(w, r.WithContext(ctx), d)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func failed(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// pair takes a pairing code from a phone and holds it until the user answers on
// this computer.
func (s *Server) pair(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		failed(w, http.StatusBadRequest, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.openPairing()
	if p == nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no pairing is open; start one in Freeloop's Settings, Phone"})
		return
	}
	if p.claim != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "another phone is already waiting for this code"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(normalCode(body.Code)), []byte(p.code)) != 1 {
		p.tries++
		if p.tries >= pairTries {
			s.pairing = nil
		}
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "that code is not the one Freeloop shows"})
		return
	}
	name := cleanName(body.Name)
	if name == "" {
		name = "Phone"
	}
	p.claim = &claim{id: newID(), name: name, login: r.Header.Get(tsLogin), claimed: s.now()}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": p.claim.id})
}

// pairAnswer tells a waiting phone the user's answer, and with "allowed" its key,
// once.
func (s *Server) pairAnswer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	var c *claim
	if s.pairing != nil && s.pairing.claim != nil && s.pairing.claim.id == id {
		c = s.pairing.claim
	}
	var token string
	answer := ""
	if c != nil {
		answer, token = c.answer, c.token
		if answer == "allowed" {
			s.pairing = nil // the key goes out once
		}
	}
	s.mu.Unlock()
	switch {
	case c == nil:
		writeJSON(w, http.StatusOK, map[string]string{"answer": "refused"})
	case answer == "allowed":
		// The phone app keeps the key in its own storage and sends it as a header.
		writeJSON(w, http.StatusOK, map[string]string{"answer": "allowed", "token": token})
	default:
		writeJSON(w, http.StatusOK, map[string]string{"answer": "waiting"})
	}
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, d config.Device) {
	if err := s.RemoveDevice(d.ID); err != nil {
		failed(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func via(d config.Device) string { return "phone:" + d.Name }

func (s *Server) summary(w http.ResponseWriter, r *http.Request, _ config.Device) {
	sum, err := s.eng.PhoneSummary(r.Context())
	if err != nil {
		failed(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

// readable finds a session the phone may read.
func (s *Server) readable(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	ses, err := s.eng.GetSession(r.Context(), id)
	if err != nil || ses.ParentID != "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such session"})
		return "", false
	}
	return id, true
}

func (s *Server) events(w http.ResponseWriter, r *http.Request, _ config.Device) {
	id, ok := s.readable(w, r)
	if !ok {
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > engine.MaxEventsPage {
		limit = engine.MaxEventsPage
	}
	evs, err := s.eng.Events(r.Context(), id, after, limit)
	if err != nil {
		failed(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, evs)
}

func (s *Server) prompt(w http.ResponseWriter, r *http.Request, d config.Device) {
	id, ok := s.readable(w, r)
	if !ok {
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "write a message"})
		return
	}
	res, err := s.eng.PromptFrom(context.WithoutCancel(r.Context()), id, body.Text, via(d))
	if err != nil {
		failed(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) cancel(w http.ResponseWriter, r *http.Request, _ config.Device) {
	id, ok := s.readable(w, r)
	if !ok {
		return
	}
	if err := s.eng.Cancel(id); err != nil {
		failed(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) decide(w http.ResponseWriter, r *http.Request, d config.Device) {
	var body struct {
		RequestID string `json:"requestId"`
		OptionID  string `json:"optionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		failed(w, http.StatusBadRequest, err)
		return
	}
	switch err := s.eng.DecideOnce(body.RequestID, body.OptionID, via(d)); {
	case errors.Is(err, engine.ErrDecideOnLaptop):
		failed(w, http.StatusForbidden, err)
	case err != nil:
		failed(w, http.StatusConflict, fmt.Errorf("the request was answered or withdrawn meanwhile"))
	default:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
