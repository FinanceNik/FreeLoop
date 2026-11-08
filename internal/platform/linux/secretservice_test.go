//go:build linux

package linux

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/secrets/secretstest"
)

// With no keyring the store says so, and never keeps a secret anywhere else.
func TestSecretServiceWithoutABus(t *testing.T) {
	s := &SecretService{Address: "unix:path=/nonexistent/freeloop-test-bus"}
	ref := secrets.Ref{Service: "freeloop-test", Account: "a"}
	for name, err := range map[string]error{
		"Set":      s.Set(ref, "v"),
		"Delete":   s.Delete(ref),
		"Get":      func() error { _, err := s.Get(ref); return err }(),
		"Accounts": func() error { _, err := s.Accounts("freeloop-test"); return err }(),
	} {
		if !errors.Is(err, ErrNoSecretService) || !strings.Contains(err.Error(), "install and unlock GNOME Keyring or KWallet") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestBusAddressIsTheSessionBusVariable(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/x")
	if got := busAddress(); got != "unix:path=/x" {
		t.Errorf("got %q", got)
	}
}

// The tests below write to a real Secret Service (items under the services
// "freeloop-test" and "freeloop-test-vault", removed afterwards), so they run only when
// asked for. In the Linux image, make linux-test-keyring does:
//
//	dbus-run-session -- sh -c 'echo "" | gnome-keyring-daemon --unlock --components=secrets >/dev/null;
//	  FREELOOP_KEYRING_TEST=1 go test -count=1 -run "^TestSecretService" ./internal/platform/linux/'
//
// gnome-keyring-daemon --unlock creates the login keyring with an empty password and
// opens it, which a desktop session does at login. On a desktop: FREELOOP_KEYRING_TEST=1
// go test -run '^TestSecretService' ./internal/platform/linux/ (a locked keyring asks
// for its password).
func keyringTest(t *testing.T) *SecretService {
	if os.Getenv("FREELOOP_KEYRING_TEST") != "1" {
		t.Skip("set FREELOOP_KEYRING_TEST=1 to run against the Secret Service")
	}
	return NewSecretService()
}

func TestSecretService(t *testing.T) {
	secretstest.Run(t, keyringTest(t), "freeloop-test")
}

// Items hold the attributes that name them, so other tools find them too.
func TestSecretServiceItemsCarryServiceAndAccount(t *testing.T) {
	s := keyringTest(t)
	ref := secrets.Ref{Service: "freeloop-test", Account: "providers/a b"}
	if err := s.Set(ref, "k"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Delete(ref) })
	accounts, err := s.Accounts("freeloop-test")
	if err != nil || len(accounts) != 1 || accounts[0] != ref.Account {
		t.Fatalf("accounts %v, %v", accounts, err)
	}
	if err := s.Set(ref, "k2"); err != nil {
		t.Fatal(err)
	}
	if accounts, _ := s.Accounts("freeloop-test"); len(accounts) != 1 {
		t.Errorf("a second Set made a second item: %v", accounts)
	}
}

// The vault on the real keyring, moving separate items in, under a service of its own
// that it removes again.
func TestSecretServiceVault(t *testing.T) {
	const service = "freeloop-test-vault"
	k := keyringTest(t)
	clean := func() {
		accounts, err := k.Accounts(service)
		if err != nil {
			t.Error(err)
		}
		for _, a := range accounts {
			if err := k.Delete(secrets.Ref{Service: service, Account: a}); err != nil {
				t.Error(err)
			}
		}
	}
	clean()
	t.Cleanup(clean)
	items := map[string]string{"providers/a": "key a", "environments/e/n": "value é", "app/commands-adopted//tmp/x": "m"}
	for account, value := range items {
		if err := k.Set(secrets.Ref{Service: service, Account: account}, value); err != nil {
			t.Fatal(err)
		}
	}
	if accounts, err := k.Accounts(service); err != nil || len(accounts) != len(items) {
		t.Fatalf("listed %v, %v", accounts, err)
	}
	v := secrets.NewVault(k, service)
	for account, value := range items {
		if got, err := v.Get(secrets.Ref{Service: service, Account: account}); err != nil || got != value {
			t.Fatalf("%s: %q, %v", account, got, err)
		}
	}
	accounts, err := k.Accounts(service)
	if err != nil || len(accounts) != 1 || accounts[0] != secrets.VaultAccount {
		t.Fatalf("left after the move: %v, %v", accounts, err)
	}
	secretstest.Run(t, secrets.NewVault(k, service), service)
}
