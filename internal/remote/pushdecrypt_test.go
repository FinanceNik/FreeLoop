package remote

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/sha256"
	"errors"
)

// decryptPush is what the phone's browser does: RFC 8291's receiving side.
func decryptPush(msg []byte, uaPriv *ecdh.PrivateKey, auth []byte) ([]byte, error) {
	if len(msg) < 21 {
		return nil, errors.New("short")
	}
	salt, idlen := msg[:16], int(msg[20])
	if len(msg) < 21+idlen {
		return nil, errors.New("short")
	}
	asPub, err := ecdh.P256().NewPublicKey(msg[21 : 21+idlen])
	if err != nil {
		return nil, err
	}
	secret, err := uaPriv.ECDH(asPub)
	if err != nil {
		return nil, err
	}
	prkKey, _ := hkdf.Extract(sha256.New, secret, auth)
	info := append(append([]byte("WebPush: info\x00"), uaPriv.PublicKey().Bytes()...), asPub.Bytes()...)
	ikm, _ := hkdf.Expand(sha256.New, prkKey, string(info), 32)
	prk, _ := hkdf.Extract(sha256.New, ikm, salt)
	cek, _ := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, msg[21+idlen:], nil)
	if err != nil {
		return nil, err
	}
	if len(plain) == 0 || plain[len(plain)-1] != 2 {
		return nil, errors.New("not a last record")
	}
	return plain[:len(plain)-1], nil
}
