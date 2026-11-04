//go:build darwin

package darwin

import (
	"os"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/secrets/secretstest"
)

// The test writes to the real login keychain (an item under the service
// "freeloop-test", removed afterwards), so it only runs when asked for.
func TestKeychain(t *testing.T) {
	if os.Getenv("FREELOOP_KEYCHAIN_TEST") != "1" {
		t.Skip("set FREELOOP_KEYCHAIN_TEST=1 to run against the login keychain")
	}
	secretstest.Run(t, Keychain{}, "freeloop-test")
}

// The vault on the real login keychain, moving separate items in, under a service of its
// own that it removes again.
func TestKeychainVault(t *testing.T) {
	if os.Getenv("FREELOOP_KEYCHAIN_TEST") != "1" {
		t.Skip("set FREELOOP_KEYCHAIN_TEST=1 to run against the login keychain")
	}
	const service = "freeloop-test-vault"
	k := Keychain{}
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
	if accounts, err := k.Accounts(service); err != nil || len(accounts) != 0 {
		t.Fatalf("not empty: %v, %v", accounts, err)
	}
	items := map[string]string{"providers/a": "key a", "environments/e/n": "value \u00e9", "app/commands-adopted//tmp/x": "m"}
	for account, value := range items {
		if err := k.Set(secrets.Ref{Service: service, Account: account}, value); err != nil {
			t.Fatal(err)
		}
	}
	accounts, err := k.Accounts(service)
	if err != nil || len(accounts) != len(items) {
		t.Fatalf("listed %v, %v", accounts, err)
	}
	v := secrets.NewVault(k, service)
	for account, value := range items {
		if got, err := v.Get(secrets.Ref{Service: service, Account: account}); err != nil || got != value {
			t.Fatalf("%s: %q, %v", account, got, err)
		}
	}
	accounts, err = k.Accounts(service)
	if err != nil || len(accounts) != 1 || accounts[0] != secrets.VaultAccount {
		t.Fatalf("left after the move: %v, %v", accounts, err)
	}
	secretstest.Run(t, secrets.NewVault(k, service), service)
	fresh := secrets.NewVault(k, service)
	if got, err := fresh.Get(secrets.Ref{Service: service, Account: "providers/a"}); err != nil || got != "key a" {
		t.Fatalf("a new start: %q, %v", got, err)
	}
}
