package remote

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// Sending a Web Push message (GUI-052, issue 028): RFC 8291 encryption, RFC 8188
// framing and an RFC 8292 VAPID token, all from the standard library. Which sessions
// push, and when, is issue 029.

// pushSubject is the VAPID `sub` claim: the project's URL, which a push service may
// use to reach whoever runs the sender. Apple refuses a push without a valid one.
const pushSubject = "https://github.com/FinanceNik/Freeloop"

// pushTTL is how long a push service keeps a message for a phone that is off.
const pushTTL = time.Hour

// vapidLife is how long a token lasts; RFC 8292 allows at most 24 hours.
const vapidLife = 11 * time.Hour

// newPushClient makes the client for push services. It uses http.DefaultTransport,
// so local-only mode's guard applies to it, and it never follows a redirect: the
// endpoint was checked, wherever it might send us was not.
func newPushClient() *http.Client {
	return &http.Client{
		Transport:     http.DefaultTransport,
		Timeout:       20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// pushPayload is all a push carries: what happened and which session. Nothing of
// the session's content leaves this computer (SEC-009).
type pushPayload struct {
	Kind      string `json:"kind"`
	SessionID string `json:"sessionId"`
}

// Push sends the phone a Web Push message of the given kind about a session. The
// subscription's endpoint is checked again, nothing is sent in local-only mode, and
// a 404 or 410 from the push service drops the subscription.
func (s *Server) Push(deviceID, kind, sessionID string) error {
	if netguard.LocalOnly() {
		return errors.New("local-only mode: nothing is sent to a push service")
	}
	if err := s.pushable(); err != nil {
		return err
	}
	var sub *config.PushSubscription
	s.mu.Lock()
	for _, d := range s.devices {
		if d.ID == deviceID && d.Push != nil {
			c := *d.Push
			sub = &c
		}
	}
	client := s.pushClient
	s.mu.Unlock()
	if sub == nil {
		return errors.New("that phone has not turned notifications on")
	}
	if err := checkSubscription(*sub); err != nil {
		return err
	}
	if client == nil {
		client = newPushClient()
	}
	key, err := s.vapidKey()
	if err != nil {
		return err
	}
	body, err := json.Marshal(pushPayload{Kind: kind, SessionID: sessionID})
	if err != nil {
		return err
	}
	ua, _ := rawKey.DecodeString(sub.P256dh)
	auth, _ := rawKey.DecodeString(sub.Auth)
	ephemeral, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	msg, err := encryptPush(body, ua, auth, ephemeral, salt)
	if err != nil {
		return err
	}
	u, _ := url.Parse(sub.Endpoint)
	authz, err := vapidHeader(key, u.Scheme+"://"+u.Host, s.now().Add(vapidLife))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(msg))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", strconv.Itoa(int(pushTTL/time.Second)))
	req.Header.Set("Urgency", "high")
	req.Header.Set("Authorization", authz)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		if err := s.dropPush(deviceID, sub.Endpoint); err != nil {
			return err
		}
		return fmt.Errorf("push: the push service no longer knows this subscription (%d); dropped", resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return fmt.Errorf("push: the push service answered %d", resp.StatusCode)
	}
	return nil
}

// dropPush forgets the phone's subscription, if it is still the one that failed.
func (s *Server) dropPush(id, endpoint string) error {
	s.mu.Lock()
	for _, d := range s.devices {
		if d.ID == id && (d.Push == nil || d.Push.Endpoint != endpoint) {
			s.mu.Unlock()
			return nil
		}
	}
	s.mu.Unlock()
	return s.setPush(id, nil)
}

// vapidHeader is the Authorization header of RFC 8292: a JWT signed ES256 (raw r||s)
// for the endpoint's origin, and the public key.
func vapidHeader(key *ecdsa.PrivateKey, aud string, exp time.Time) (string, error) {
	claims, err := json.Marshal(map[string]any{"aud": aud, "exp": exp.Unix(), "sub": pushSubject})
	if err != nil {
		return "", err
	}
	signing := rawKey.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`)) + "." + rawKey.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	der, err := ecdsa.SignASN1(rand.Reader, key, sum[:])
	if err != nil {
		return "", err
	}
	var rs struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(der, &rs); err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	rs.R.FillBytes(sig[:32])
	rs.S.FillBytes(sig[32:])
	pub, err := key.PublicKey.ECDH()
	if err != nil {
		return "", err
	}
	return "vapid t=" + signing + "." + rawKey.EncodeToString(sig) + ", k=" + rawKey.EncodeToString(pub.Bytes()), nil
}

// pushRecord is the record size written in the header; a payload here is far
// smaller, so it is always one record.
const pushRecord = 4096

// encryptPush is RFC 8291's aes128gcm: plaintext for the phone's key ua (65 bytes)
// and auth secret, from the sender's one-time key and a salt, framed per RFC 8188 as
// salt, record size, key ID length, the sender's public key and the single, last
// record.
func encryptPush(plaintext, ua, auth []byte, sender *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	if len(plaintext) > pushRecord-86-17 {
		return nil, errors.New("push payload too large")
	}
	uaKey, err := ecdh.P256().NewPublicKey(ua)
	if err != nil {
		return nil, err
	}
	secret, err := sender.ECDH(uaKey)
	if err != nil {
		return nil, err
	}
	as := sender.PublicKey().Bytes()
	prkKey, err := hkdf.Extract(sha256.New, secret, auth)
	if err != nil {
		return nil, err
	}
	info := append(append([]byte("WebPush: info\x00"), ua...), as...)
	ikm, err := hkdf.Expand(sha256.New, prkKey, string(info), 32)
	if err != nil {
		return nil, err
	}
	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	out := append([]byte{}, salt...)
	out = binary.BigEndian.AppendUint32(out, pushRecord)
	out = append(out, byte(len(as)))
	out = append(out, as...)
	// 0x02 marks the last record (RFC 8188 section 2).
	return gcm.Seal(out, nonce, append(append([]byte{}, plaintext...), 2), nil), nil
}
