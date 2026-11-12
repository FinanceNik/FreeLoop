package remote

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
)

func dec(t *testing.T, s string) []byte {
	t.Helper()
	b, err := b64.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// 028: RFC 8291 section 5's example: its keys, salt and plaintext give its ciphertext.
func TestEncryptMatchesRFC8291(t *testing.T) {
	asPriv, err := ecdh.P256().NewPrivateKey(dec(t, "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := encryptPush([]byte("When I grow up, I want to be a watermelon"),
		dec(t, "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"),
		dec(t, "BTBZMqHH6r4Tts7J_aSIgg"), asPriv, dec(t, "DGv6ra1nlYgDCS1FRnbzlw"))
	if err != nil {
		t.Fatal(err)
	}
	want := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if b64.EncodeToString(got) != want {
		t.Fatalf("ciphertext\n got %s\nwant %s", b64.EncodeToString(got), want)
	}
	// And the phone's side reads it back.
	uaPriv, _ := ecdh.P256().NewPrivateKey(dec(t, "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"))
	plain, err := decryptPush(got, uaPriv, dec(t, "BTBZMqHH6r4Tts7J_aSIgg"))
	if err != nil || string(plain) != "When I grow up, I want to be a watermelon" {
		t.Fatalf("decrypt: %q %v", plain, err)
	}
}

func verifyVAPID(t *testing.T, header string, pub *ecdsa.PublicKey) map[string]any {
	t.Helper()
	rest, ok := strings.CutPrefix(header, "vapid t=")
	if !ok {
		t.Fatalf("authorization: %q", header)
	}
	jwt, k, ok := strings.Cut(rest, ", k=")
	if !ok {
		t.Fatalf("authorization: %q", header)
	}
	pk, _ := pub.ECDH()
	if k != b64.EncodeToString(pk.Bytes()) {
		t.Errorf("k is %q", k)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("token: %q", jwt)
	}
	var hdr map[string]string
	json.Unmarshal(dec(t, parts[0]), &hdr)
	if hdr["alg"] != "ES256" || hdr["typ"] != "JWT" {
		t.Errorf("header: %v", hdr)
	}
	sig := dec(t, parts[2])
	if len(sig) != 64 {
		t.Fatalf("signature is %d bytes, not raw r||s of 64", len(sig))
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(pub, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("the signature does not verify")
	}
	var claims map[string]any
	json.Unmarshal(dec(t, parts[1]), &claims)
	return claims
}

// pushed is a fake push service; the client sends everything to it, whatever the
// endpoint says, and the endpoint is still checked as for a real one.
type pushed struct {
	got  chan *http.Request
	body chan []byte
}

func fakePush(t *testing.T, s *Server, status int) *pushed {
	t.Helper()
	f := &pushed{got: make(chan *http.Request, 4), body: make(chan []byte, 4)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.got <- r
		f.body <- b
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	s.pushClient = &http.Client{Transport: rewrite{u.Host}}
	return f
}

type rewrite struct{ host string }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	c := req.Clone(req.Context())
	c.URL.Scheme, c.URL.Host = "http", r.host
	return http.DefaultTransport.RoundTrip(c)
}

func subscribed(t *testing.T, s *Server, p *phone, endpoint string) (string, *ecdh.PrivateKey, []byte) {
	t.Helper()
	pairPhone(t, s, p)
	k, _ := ecdh.P256().GenerateKey(bytes.NewReader(bytes.Repeat([]byte{7}, 1024)))
	auth := bytes.Repeat([]byte{9}, 16)
	sub := map[string]any{"endpoint": endpoint, "keys": map[string]string{"p256dh": b64.EncodeToString(k.PublicKey().Bytes()), "auth": b64.EncodeToString(auth)}}
	if resp, out := p.do("POST", "/api/push/subscribe", sub, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("subscribe: %d %v", resp.StatusCode, out)
	}
	return pairedID(s), k, auth
}

// 028: the request carries TTL, aes128gcm and a VAPID authorization with the claims
// 8292 asks for; the body decrypts to the kind and session ID and nothing else.
func TestPushSendsAnEncryptedMessage(t *testing.T) {
	_, s, p := started(t)
	f := fakePush(t, s, http.StatusCreated)
	id, ua, auth := subscribed(t, s, p, "https://fcm.googleapis.com/fcm/send/abc")
	if err := s.Push(id, "approval", "ses_123"); err != nil {
		t.Fatal(err)
	}
	r := <-f.got
	body := <-f.body
	if r.Method != "POST" || r.URL.Path != "/fcm/send/abc" {
		t.Errorf("request: %s %s", r.Method, r.URL)
	}
	if r.Header.Get("TTL") == "" || r.Header.Get("Content-Encoding") != "aes128gcm" || r.Header.Get("Content-Type") != "application/octet-stream" {
		t.Errorf("headers: %v", r.Header)
	}
	key, _ := s.vapidKey()
	claims := verifyVAPID(t, r.Header.Get("Authorization"), &key.PublicKey)
	if claims["aud"] != "https://fcm.googleapis.com" {
		t.Errorf("aud: %v", claims["aud"])
	}
	exp, _ := claims["exp"].(float64)
	if d := time.Until(time.Unix(int64(exp), 0)); d <= 0 || d > 12*time.Hour {
		t.Errorf("exp is %v away", d)
	}
	if sub, _ := claims["sub"].(string); !strings.HasPrefix(sub, "https://") {
		t.Errorf("sub: %v", claims["sub"])
	}
	plain, err := decryptPush(body, ua, auth)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != `{"kind":"approval","sessionId":"ses_123"}` {
		t.Errorf("payload: %s", plain)
	}
	if bytes.Contains(body, []byte("ses_123")) {
		t.Error("the payload is not encrypted")
	}
}

// 028: a push service that no longer knows the subscription drops it.
func TestPushGoneDropsTheSubscription(t *testing.T) {
	for _, status := range []int{http.StatusGone, http.StatusNotFound} {
		_, s, p := started(t)
		fakePush(t, s, status)
		id, _, _ := subscribed(t, s, p, "https://updates.push.services.mozilla.com/wpush/v2/abc")
		if err := s.Push(id, "done", "ses_1"); err == nil {
			t.Errorf("%d: no error", status)
		}
		if notifies(s, id) {
			t.Errorf("%d: the subscription was kept", status)
		}
		if d, _ := config.LoadDevices(s.dataDir); d[0].Push != nil {
			t.Errorf("%d: not dropped from devices.json", status)
		}
	}
}

// 028: another failure keeps the subscription; a phone without one gets nothing.
func TestPushOtherFailuresKeepIt(t *testing.T) {
	_, s, p := started(t)
	f := fakePush(t, s, http.StatusServiceUnavailable)
	id, _, _ := subscribed(t, s, p, "https://web.push.apple.com/abc")
	if err := s.Push(id, "done", "ses_1"); err == nil {
		t.Error("a 503 was not an error")
	}
	<-f.got
	if !notifies(s, id) {
		t.Error("a 503 dropped the subscription")
	}
	p.do("POST", "/api/push/unsubscribe", map[string]any{}, nil)
	if err := s.Push(id, "done", "ses_1"); err == nil {
		t.Error("sent to a phone with push off")
	}
	select {
	case <-f.got:
		t.Error("a request went out for a phone with push off")
	default:
	}
}

// 028: local-only mode sends nothing.
func TestPushSendsNothingInLocalOnlyMode(t *testing.T) {
	e, s, p := started(t)
	f := fakePush(t, s, http.StatusCreated)
	id, _, _ := subscribed(t, s, p, "https://fcm.googleapis.com/fcm/send/abc")
	cfg := e.Config()
	cfg.Network.LocalOnly = true
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	if err := s.Push(id, "done", "ses_1"); err == nil {
		t.Error("no error in local-only mode")
	}
	select {
	case <-f.got:
		t.Error("a request went out in local-only mode")
	default:
	}
}

// 028: a stored endpoint that is not on the list (a hand-edited devices.json) is
// never contacted, and a redirect is never followed.
func TestPushOnlyGoesToTheKnownServices(t *testing.T) {
	_, s, p := started(t)
	f := fakePush(t, s, http.StatusCreated)
	id, _, _ := subscribed(t, s, p, "https://fcm.googleapis.com/fcm/send/abc")
	s.mu.Lock()
	s.devices[0].Push.Endpoint = "https://example.com/x"
	s.mu.Unlock()
	if err := s.Push(id, "done", "ses_1"); err == nil {
		t.Error("sent to an unlisted host")
	}
	select {
	case <-f.got:
		t.Error("a request went out to an unlisted host")
	default:
	}
	c := newPushClient()
	if c.CheckRedirect == nil || c.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Error("redirects are followed")
	}
}
