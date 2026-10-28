//go:build darwin

package darwin

import (
	"os"
	"testing"

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
