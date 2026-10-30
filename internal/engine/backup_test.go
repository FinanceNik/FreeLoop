package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
)

// SET-005: restoring a backup on a fresh install recreates the providers,
// asking for the keys the Keychain does not have, and the profiles; the
// backup holds no secret.
func TestConfigBackupRestore(t *testing.T) {
	ctx := context.Background()
	a := openTest(t, t.TempDir())
	defer a.Close()
	if _, err := a.SaveProvider(SaveProviderRequest{Name: "Work", Type: "anthropic", APIKey: "sk-ant-secret-1234567890"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveProfile(config.Profile{ID: "review", Name: "Review", Mode: "plan"}); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "freeloop-config.json")
	if err := a.ExportConfig(ctx, backup); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(backup)
	if strings.Contains(string(b), "sk-ant-secret") || !strings.Contains(string(b), "keychain://") {
		t.Fatalf("the backup:\n%s", b)
	}

	fresh := openTest(t, t.TempDir())
	defer fresh.Close()
	res, err := fresh.ImportConfig(ctx, backup)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(fresh.ListProviders(), func(p ProviderInfo) bool { return p.Name == "Work" }) {
		t.Errorf("providers = %+v", fresh.ListProviders())
	}
	if !slices.ContainsFunc(fresh.ListProfiles(), func(p config.Profile) bool { return p.ID == "review" && p.Mode == "plan" }) {
		t.Errorf("profiles = %+v", fresh.ListProfiles())
	}
	if !slices.Equal(res.NeedKeys, []string{"Work"}) {
		t.Errorf("need keys = %v", res.NeedKeys)
	}
}
