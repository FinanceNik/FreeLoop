package release

import (
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"
)

func testKey(t *testing.T, seed byte) (ed25519.PrivateKey, string) {
	t.Helper()
	key := ed25519.NewKeyFromSeed([]byte(strings.Repeat(string(seed), ed25519.SeedSize)))
	return key, base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
}

// SEC-021: a signature holds for the version and the list it was made over, and
// for nothing else.
func TestVerify(t *testing.T) {
	key, pub := testKey(t, 'a')
	_, other := testKey(t, 'b')
	sums := []byte("0000000000000000000000000000000000000000000000000000000000000000  Freeloop.dmg\n")
	sig := Sign(key, "0.2.0", sums)

	if err := Verify([]string{pub}, "0.2.0", sums, sig); err != nil {
		t.Errorf("the project's own release: %v", err)
	}
	// A key being replaced: either one signs.
	if err := Verify([]string{other, pub}, "0.2.0", sums, sig); err != nil {
		t.Errorf("with two keys: %v", err)
	}
	for name, err := range map[string]error{
		"another key": Verify([]string{other}, "0.2.0", sums, sig),
		"an older release's files as a newer one": Verify([]string{pub}, "0.3.0", sums, sig),
		"a changed list":        Verify([]string{pub}, "0.2.0", append([]byte("x"), sums...), sig),
		"no signature":          Verify([]string{pub}, "0.2.0", sums, ""),
		"a signature cut short": Verify([]string{pub}, "0.2.0", sums, sig[:20]),
		"a key that is not one": Verify([]string{"nonsense"}, "0.2.0", sums, sig),
	} {
		if err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if err := Verify(nil, "0.2.0", sums, sig); err != ErrUnsigned {
		t.Errorf("a copy without a key: %v", err)
	}
}

func TestKeysRoundTrip(t *testing.T) {
	seed := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", ed25519.SeedSize)))
	key, err := ParsePrivateKey(seed + "\n")
	if err != nil {
		t.Fatal(err)
	}
	pub := base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
	if _, err := ParsePublicKey(pub); err != nil {
		t.Error(err)
	}
	if _, err := ParsePrivateKey(pub + "x"); err == nil {
		t.Error("a key that is not one was read")
	}
	// The app's own keys, when it has any, are keys.
	for _, k := range Keys {
		if _, err := ParsePublicKey(k); err != nil {
			t.Errorf("Keys holds %q: %v", k, err)
		}
	}
}

func TestSums(t *testing.T) {
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	got, err := Sums([]byte(a + "  Freeloop.dmg\n" + b + "  *BUILDINFO.txt\n"))
	if err != nil || got["Freeloop.dmg"] != a || got["BUILDINFO.txt"] != b || len(got) != 2 {
		t.Errorf("sums = %v, %v", got, err)
	}
	for name, bad := range map[string]string{
		"a short checksum":    "abc  Freeloop.dmg\n",
		"a path":              a + "  ../Freeloop.dmg\n",
		"a Windows path":      a + "  x\\Freeloop.dmg\n",
		"a name listed twice": a + "  Freeloop.dmg\n" + b + "  Freeloop.dmg\n",
		"capitals":            strings.Repeat("A", 64) + "  Freeloop.dmg\n",
		"no name":             a + "  \n",
		"nothing":             "",
	} {
		if _, err := Sums([]byte(bad)); err == nil {
			t.Errorf("%s was read", name)
		}
	}
}

func TestAsset(t *testing.T) {
	for kind, want := range map[string]string{
		KindDMG:      "Freeloop.dmg",
		KindZip:      "Freeloop-0.2.0-windows-arm64-portable.zip",
		KindAppImage: "Freeloop-0.2.0-linux-arm64.AppImage",
	} {
		if got, err := Asset(kind, "0.2.0", "arm64"); err != nil || got != want {
			t.Errorf("%s = %q, %v", kind, got, err)
		}
	}
	if _, err := Asset("deb", "0.2.0", "arm64"); err == nil {
		t.Error("a .deb has no in-place update")
	}
}
