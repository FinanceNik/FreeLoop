package engine

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// NFR-OFF-003: with local-only on, a provider outside this Mac is not
// reached: the session says why and the attempt is listed; turning it off
// in Settings lifts it.
func TestLocalOnly(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	defer netguard.SetLocalOnly(false)
	p, err := e.SaveProvider(SaveProviderRequest{Name: "Remote", Type: "openai_compat", BaseURL: "http://203.0.113.9:8000/v1", DefaultModel: "m"})
	if err != nil {
		t.Fatal(err)
	}
	c := e.Config()
	c.Network.LocalOnly = true
	if _, err := e.SaveSettings(c); err != nil {
		t.Fatal(err)
	}
	if !netguard.LocalOnly() {
		t.Fatal("the setting did not turn local-only on")
	}
	_, err = e.TestProvider(ctx, p.ID)
	if err == nil || !strings.Contains(err.Error(), "local-only mode blocks the connection to 203.0.113.9") {
		t.Errorf("listing a remote provider's models = %v", err)
	}
	if !slices.ContainsFunc(e.BlockedConnections(), func(b netguard.Blocked) bool { return b.Host == "203.0.113.9" }) {
		t.Errorf("blocked = %+v", e.BlockedConnections())
	}
	c.Network.LocalOnly = false
	e.SaveSettings(c)
	if netguard.LocalOnly() {
		t.Error("turning it off did not")
	}
}
