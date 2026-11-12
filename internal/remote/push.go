package remote

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// Web Push for the phone (GUI-052, PRD decision 9): the app holds a VAPID key pair
// and each phone may keep one subscription, at a push service on a short list. What
// is sent through them is issue 029.

// vapidRef is where the private key lives: the system keyring, behind Secrets.
var vapidRef = secrets.Ref{Service: "freeloop", Account: "phone/vapid"}

var rawKey = base64.RawURLEncoding

// vapidKey is the app's VAPID key, made on first use and read from Secrets after.
func (s *Server) vapidKey() (*ecdsa.PrivateKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.vapid != nil {
		return s.vapid, nil
	}
	stored, err := s.keys.Get(vapidRef)
	var k *ecdh.PrivateKey
	switch {
	case err == nil:
		raw, derr := rawKey.DecodeString(stored)
		if derr != nil {
			return nil, fmt.Errorf("the stored push key is damaged: %w", derr)
		}
		if k, derr = ecdh.P256().NewPrivateKey(raw); derr != nil {
			return nil, fmt.Errorf("the stored push key is damaged: %w", derr)
		}
	case errors.Is(err, secrets.ErrNotFound):
		if k, err = ecdh.P256().GenerateKey(rand.Reader); err != nil {
			return nil, err
		}
		if err = s.keys.Set(vapidRef, rawKey.EncodeToString(k.Bytes())); err != nil {
			return nil, fmt.Errorf("keep the push key: %w", err)
		}
	default:
		return nil, fmt.Errorf("read the push key: %w", err)
	}
	key, err := toECDSA(k)
	if err != nil {
		return nil, err
	}
	s.vapid = key
	return key, nil
}

// toECDSA converts the key to the form that signs the VAPID token (issue 029).
func toECDSA(k *ecdh.PrivateKey) (*ecdsa.PrivateKey, error) {
	x, y := elliptic.Unmarshal(elliptic.P256(), k.PublicKey().Bytes()) //nolint:staticcheck // no ecdh-to-ecdsa conversion exists
	if x == nil {
		return nil, errors.New("push key: bad public point")
	}
	return &ecdsa.PrivateKey{PublicKey: ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, D: new(big.Int).SetBytes(k.Bytes())}, nil
}

// vapidPublic is the public key as a browser's subscribe call takes it: the 65-byte
// uncompressed point.
func (s *Server) vapidPublic() ([]byte, error) {
	k, err := s.vapidKey()
	if err != nil {
		return nil, err
	}
	pub, err := k.PublicKey.ECDH()
	if err != nil {
		return nil, err
	}
	return pub.Bytes(), nil
}

// pushable says whether the phone may use push now: not in local-only mode, and not
// while the phone server is off for any other reason.
func (s *Server) pushable() error {
	st := s.eng.RemoteState()
	if st.Why == "local-only" {
		return errors.New("local-only mode: nothing is sent to a push service")
	}
	if !st.On {
		return errors.New("the phone server is off")
	}
	return nil
}

// pushHost says whether host is a known push service: Apple's, Google's or Mozilla's.
func pushHost(host string) bool {
	host = strings.ToLower(host)
	return host == "push.apple.com" || strings.HasSuffix(host, ".push.apple.com") ||
		host == "fcm.googleapis.com" ||
		host == "updates.push.services.mozilla.com"
}

// checkSubscription is the first thing done with a subscription: the endpoint must
// be https on a known push service, and both keys must be what a browser makes (a
// P-256 point and 16 bytes). Nothing else about it is trusted.
func checkSubscription(sub config.PushSubscription) error {
	u, err := url.Parse(sub.Endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" || (u.Port() != "" && u.Port() != "443") || u.Fragment != "" {
		return errors.New("the push endpoint must be an https address of a known push service")
	}
	if !pushHost(u.Hostname()) {
		return errors.New("that is not a known push service")
	}
	p, err := rawKey.DecodeString(sub.P256dh)
	if err != nil {
		return errors.New("the push key is not valid")
	}
	if _, err := ecdh.P256().NewPublicKey(p); err != nil {
		return errors.New("the push key is not valid")
	}
	if a, err := rawKey.DecodeString(sub.Auth); err != nil || len(a) != 16 {
		return errors.New("the push secret is not valid")
	}
	return nil
}

func (s *Server) pushKey(w http.ResponseWriter, r *http.Request, _ config.Device) {
	if err := s.pushable(); err != nil {
		failed(w, http.StatusForbidden, err)
		return
	}
	pub, err := s.vapidPublic()
	if err != nil {
		failed(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": rawKey.EncodeToString(pub)})
}

func (s *Server) pushSubscribe(w http.ResponseWriter, r *http.Request, d config.Device) {
	var body struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		failed(w, http.StatusBadRequest, err)
		return
	}
	// The endpoint is checked before anything else, the mode included.
	sub := config.PushSubscription{Endpoint: body.Endpoint, P256dh: body.Keys.P256dh, Auth: body.Keys.Auth}
	if err := checkSubscription(sub); err != nil {
		failed(w, http.StatusBadRequest, err)
		return
	}
	if err := s.pushable(); err != nil {
		failed(w, http.StatusForbidden, err)
		return
	}
	if err := s.setPush(d.ID, &sub); err != nil {
		failed(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) pushUnsubscribe(w http.ResponseWriter, r *http.Request, d config.Device) {
	if err := s.setPush(d.ID, nil); err != nil {
		failed(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// setPush keeps sub as the phone's only subscription, or drops it when nil.
func (s *Server) setPush(id string, sub *config.PushSubscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := append([]config.Device{}, s.devices...)
	for i := range next {
		if next[i].ID != id {
			continue
		}
		next[i].Push = sub
		if err := config.SaveDevices(s.dataDir, next); err != nil {
			return err
		}
		s.devices = next
		return nil
	}
	return errors.New("no such phone")
}
