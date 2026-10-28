package secrets_test

import (
	"testing"

	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/secrets/secretstest"
)

func TestMemory(t *testing.T) {
	secretstest.Run(t, secrets.NewMemory(), "freeloop-test")
}

func TestParseRef(t *testing.T) {
	r, err := secrets.ParseRef("keychain://freeloop/providers/anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if r.Service != "freeloop" || r.Account != "providers/anthropic" {
		t.Errorf("ParseRef = %+v", r)
	}
	if r.String() != "keychain://freeloop/providers/anthropic" {
		t.Errorf("String = %q", r.String())
	}
	for _, bad := range []string{"", "sk-ant-123", "keychain://", "keychain://freeloop", "keychain://freeloop/", "keychain:///acct", "file://freeloop/x"} {
		if _, err := secrets.ParseRef(bad); err == nil {
			t.Errorf("ParseRef(%q) succeeded", bad)
		}
	}
}
