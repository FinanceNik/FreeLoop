package remote

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"testing"
	"testing/fstest"

	"github.com/FinanceNik/Freeloop/internal/config"
)

var b64 = base64.RawURLEncoding

// subscription is what a browser's PushSubscription.toJSON() gives: a real P-256
// key and a 16-byte secret.
func subscription(endpoint string) map[string]any {
	k, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	rand.Read(auth)
	return map[string]any{"endpoint": endpoint, "keys": map[string]string{"p256dh": b64.EncodeToString(k.PublicKey().Bytes()), "auth": b64.EncodeToString(auth)}}
}

func notifies(s *Server, id string) bool {
	for _, d := range s.Status().Devices {
		if d.ID == id {
			return d.Notify
		}
	}
	return false
}

func pairedID(s *Server) string { return s.Status().Devices[0].ID }

// 027: only a subscription at a known push service, over https, with a real key.
func TestPushRefusesWhatIsNotAPushService(t *testing.T) {
	_, s, p := started(t)
	pairPhone(t, s, p)
	bad := subscription("https://fcm.googleapis.com/fcm/send/abc")
	for name, sub := range map[string]map[string]any{
		"elsewhere":        subscription("https://example.com/push/abc"),
		"plain http":       subscription("http://fcm.googleapis.com/fcm/send/abc"),
		"look-alike":       subscription("https://fcm.googleapis.com.evil.test/x"),
		"suffix trick":     subscription("https://evilfcm.googleapis.com/x"),
		"userinfo":         subscription("https://fcm.googleapis.com@evil.test/x"),
		"another port":     subscription("https://fcm.googleapis.com:8443/x"),
		"apple look-alike": subscription("https://push.apple.com.evil.test/x"),
		"bad key":          {"endpoint": "https://fcm.googleapis.com/x", "keys": map[string]string{"p256dh": "AAAA", "auth": b64.EncodeToString(make([]byte, 16))}},
		"bad auth":         {"endpoint": bad["endpoint"], "keys": map[string]string{"p256dh": bad["keys"].(map[string]string)["p256dh"], "auth": "AA"}},
		"no keys":          {"endpoint": "https://fcm.googleapis.com/x"},
	} {
		resp, out := p.do("POST", "/api/push/subscribe", sub, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d %v", name, resp.StatusCode, out)
		}
	}
	if notifies(s, pairedID(s)) {
		t.Error("a refused subscription was kept")
	}
	for _, ok := range []string{"https://fcm.googleapis.com/fcm/send/abc", "https://web.push.apple.com/QAbc", "https://updates.push.services.mozilla.com/wpush/v2/abc"} {
		if resp, out := p.do("POST", "/api/push/subscribe", subscription(ok), nil); resp.StatusCode != http.StatusOK {
			t.Errorf("%s: %d %v", ok, resp.StatusCode, out)
		}
	}
}

// 027: a new subscription replaces the old; unsubscribing and removing the phone
// drop it.
func TestPushSubscriptionBelongsToTheDevice(t *testing.T) {
	_, s, p := started(t)
	if resp, _ := p.do("POST", "/api/push/subscribe", subscription("https://fcm.googleapis.com/x"), nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unpaired phone: %d", resp.StatusCode)
	}
	pairPhone(t, s, p)
	id := pairedID(s)
	if notifies(s, id) {
		t.Fatal("notifications are on before the phone asked")
	}
	p.do("POST", "/api/push/subscribe", subscription("https://fcm.googleapis.com/first"), nil)
	p.do("POST", "/api/push/subscribe", subscription("https://fcm.googleapis.com/second"), nil)
	d, _ := config.LoadDevices(s.dataDir)
	if len(d) != 1 || d[0].Push == nil || d[0].Push.Endpoint != "https://fcm.googleapis.com/second" || !notifies(s, id) {
		t.Fatalf("devices.json: %+v", d)
	}
	if resp, _ := p.do("POST", "/api/push/unsubscribe", map[string]string{}, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("unsubscribe: %d", resp.StatusCode)
	}
	if d, _ := config.LoadDevices(s.dataDir); d[0].Push != nil || notifies(s, id) {
		t.Fatalf("still subscribed: %+v", d)
	}
	p.do("POST", "/api/push/subscribe", subscription("https://fcm.googleapis.com/third"), nil)
	if err := s.RemoveDevice(id); err != nil {
		t.Fatal(err)
	}
	if d, _ := config.LoadDevices(s.dataDir); len(d) != 0 {
		t.Fatalf("the subscription outlived its phone: %+v", d)
	}
}

// 027: the VAPID key is made once and read back from Secrets after a restart.
func TestVAPIDKeyIsMadeOnce(t *testing.T) {
	e, s, p := started(t)
	pairPhone(t, s, p)
	_, out := p.do("GET", "/api/push/key", nil, nil)
	first, _ := out["key"].(string)
	raw, err := s.vapidPublic()
	if err != nil || len(raw) != 65 || raw[0] != 4 {
		t.Fatalf("public key: %x %v", raw, err)
	}
	if _, err := ecdh.P256().NewPublicKey(raw); err != nil {
		t.Fatal(err)
	}
	// A new server on the same secrets and data: the same key.
	s2 := New(e, s.dataDir, s.keys, fstest.MapFS{})
	raw2, err := s2.vapidPublic()
	if err != nil || string(raw2) != string(raw) {
		t.Fatalf("the key changed after a restart: %x vs %x (%v)", raw2, raw, err)
	}
	if first != b64.EncodeToString(raw) {
		t.Errorf("GET /api/push/key says %q, not the stored key", first)
	}
}

// 027: local-only mode refuses subscribing.
func TestPushRefusedInLocalOnlyMode(t *testing.T) {
	e, s, p := started(t)
	pairPhone(t, s, p)
	cfg := e.Config()
	cfg.Network.LocalOnly = true
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	if resp, out := p.do("POST", "/api/push/subscribe", subscription("https://fcm.googleapis.com/x"), nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("subscribe in local-only mode: %d %v", resp.StatusCode, out)
	}
	if notifies(s, pairedID(s)) {
		t.Error("kept a subscription in local-only mode")
	}
}
