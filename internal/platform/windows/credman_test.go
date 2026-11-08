//go:build windows

package windows

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// The Credential Manager stores, lists, reads back and deletes a secret, a long one in
// parts. It writes real credentials of the signed-in user under the service
// freeloop-test, so it runs only when asked (FREELOOP_TEST_CREDMAN=1), as the Keychain
// test does.
func TestCredentialManager(t *testing.T) {
	if os.Getenv("FREELOOP_TEST_CREDMAN") != "1" {
		t.Skip("set FREELOOP_TEST_CREDMAN=1 to use the real Credential Manager")
	}
	s := CredentialManager{}
	short := secrets.Ref{Service: "freeloop-test", Account: "providers/short"}
	long := secrets.Ref{Service: "freeloop-test", Account: "vault"}
	t.Cleanup(func() { s.Delete(short); s.Delete(long) })
	big := strings.Repeat("0123456789", 900)
	for ref, v := range map[secrets.Ref]string{short: "sk-test", long: big} {
		if err := s.Set(ref, v); err != nil {
			t.Fatal(err)
		}
		if got, err := s.Get(ref); err != nil || got != v {
			t.Fatalf("Get %s: %d bytes, %v", ref, len(got), err)
		}
	}
	accounts, err := s.Accounts("freeloop-test")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(accounts)
	if want := []string{"providers/short", "vault"}; !slices.Equal(accounts, want) {
		t.Errorf("Accounts = %q, want %q", accounts, want)
	}
	if err := s.Set(long, "now short"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(long); got != "now short" {
		t.Errorf("after shortening, Get = %q", got)
	}
	if _, _, err := read(partName(target(long), 2)); !errors.Is(err, secrets.ErrNotFound) {
		t.Errorf("a part of the longer value was left: %v", err)
	}
	if err := s.Delete(short); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(short); !errors.Is(err, secrets.ErrNotFound) {
		t.Errorf("after Delete, Get: %v", err)
	}
}
