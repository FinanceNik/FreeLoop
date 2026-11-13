// Package release says which files a release holds and whether they are the
// project's own (SEC-021). A release's SHA256SUMS lists every file with its
// checksum; SHA256SUMS.sig is an Ed25519 signature over the release's version
// and that list, made with a key only the project holds. The app carries the
// public half, so a file it downloads to update itself (SET-008) is one the
// project released under that version, wherever it was downloaded from.
//
// The package reads no file and opens no connection: it works on bytes.
package release

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// SumsName and SigName are the two files of a release the app reads first.
const (
	SumsName = "SHA256SUMS"
	SigName  = "SHA256SUMS.sig"
)

// Keys are the public keys releases are signed with, base64 of the 32 bytes.
// More than one lets a key be replaced: a release signed with the new key is
// accepted by apps that already carry it, and the old one is removed a
// release later. A copy with none never updates itself in place and says so.
var Keys = []string{
	// The project's first key, made 2026-11-12.
	"5lOWQiyRuJr5ODQ2M87ARPrS5fg2iZguTmthvnmTtiw=",
}

// domain separates these signatures from any other use of the key.
const domain = "freeloop-release-v1\n"

// message is what is signed: the version too, so that an older release's
// files, validly signed in their day, cannot be passed off as a newer one.
func message(version string, sums []byte) []byte {
	return append([]byte(domain+version+"\n"), sums...)
}

// Sign returns the signature for a release's version and SHA256SUMS, as the
// one base64 line SHA256SUMS.sig holds.
func Sign(key ed25519.PrivateKey, version string, sums []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(key, message(version, sums))) + "\n"
}

// ErrUnsigned is a copy of the app that carries no key to check a release with.
var ErrUnsigned = errors.New("this copy of Freeloop was built without a release key, so it cannot check a download")

// Verify reports whether sig is a signature by one of keys over version and
// sums.
func Verify(keys []string, version string, sums []byte, sig string) error {
	if len(keys) == 0 {
		return ErrUnsigned
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sig))
	if err != nil || len(raw) != ed25519.SignatureSize {
		return errors.New("the release's signature is not a signature")
	}
	for _, k := range keys {
		pub, err := ParsePublicKey(k)
		if err != nil {
			continue
		}
		if ed25519.Verify(pub, message(version, sums), raw) {
			return nil
		}
	}
	return fmt.Errorf("the files of release %s are not signed by the Freeloop project", version)
}

// ParsePublicKey reads a key as Keys holds it.
func ParsePublicKey(s string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("not a release public key")
	}
	return ed25519.PublicKey(raw), nil
}

// ParsePrivateKey reads a signing key as freeloop-sign keygen writes it:
// base64 of the 32-byte seed.
func ParsePrivateKey(s string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(raw) != ed25519.SeedSize {
		return nil, errors.New("not a release signing key")
	}
	return ed25519.NewKeyFromSeed(raw), nil
}

// Sums reads a SHA256SUMS file into checksums by file name. A line it cannot
// read, a name with a folder in it or a name listed twice fails the lot: the
// file is the project's own and has none of those.
func Sums(sums []byte) (map[string]string, error) {
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimRight(string(sums), "\n"), "\n") {
		sum, name, ok := strings.Cut(line, "  ")
		name = strings.TrimPrefix(name, "*")
		if !ok || len(sum) != 64 || strings.Trim(sum, "0123456789abcdef") != "" || name == "" || strings.ContainsAny(name, `/\`) {
			return nil, fmt.Errorf("the release's checksums have a line that is not one: %q", line)
		}
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("the release's checksums list %s twice", name)
		}
		out[name] = sum
	}
	return out, nil
}

// Kinds of release file a copy of the app can replace itself with.
const (
	KindDMG      = "dmg"      // a Mac's app bundle, from the disk image
	KindZip      = "zip"      // Windows, installed or unpacked: the exe from the portable zip
	KindAppImage = "appimage" // Linux, run as an AppImage
)

// Asset is the name of the release file of a kind for a version and an
// architecture (GOARCH), as make release names them.
func Asset(kind, version, arch string) (string, error) {
	switch kind {
	case KindDMG:
		return "Freeloop.dmg", nil
	case KindZip:
		return fmt.Sprintf("Freeloop-%s-windows-%s-portable.zip", version, arch), nil
	case KindAppImage:
		return fmt.Sprintf("Freeloop-%s-linux-%s.AppImage", version, arch), nil
	}
	return "", fmt.Errorf("no release file of kind %q", kind)
}
