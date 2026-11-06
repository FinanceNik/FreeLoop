package engine

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// The version the app reports is the one every build file gives it. A release that
// bumps one and not the others would tell its users to update forever.
func TestAppVersionMatchesTheBuild(t *testing.T) {
	// macOS takes only numbers in a bundle's versions, so a pre-release's bundle gives
	// the release it comes before: 0.1.0 for 0.1.0-beta.1.
	bundleVersion, _, _ := strings.Cut(AppVersion, "-")
	for file, want := range map[string]string{
		"../../build/config.yml":            `version: "` + AppVersion + `"`,
		"../../build/darwin/Info.plist":     `<string>` + bundleVersion + `</string>`,
		"../../build/darwin/Info.dev.plist": `<string>` + bundleVersion + `</string>`,
		"../../frontend/package.json":       `"version": "` + AppVersion + `"`,
	} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), want) {
			t.Errorf("%s does not give version %s", file, AppVersion)
		}
	}
	// CFBundleVersion and CFBundleShortVersionString, in both.
	for _, file := range []string{"../../build/darwin/Info.plist", "../../build/darwin/Info.dev.plist"} {
		b, _ := os.ReadFile(file)
		if n := strings.Count(string(b), `<string>`+bundleVersion+`</string>`); n != 2 {
			t.Errorf("%s gives version %s %d times", file, bundleVersion, n)
		}
	}
}

func TestVersionOrder(t *testing.T) {
	for _, c := range []struct {
		a, b  string
		newer bool
	}{{"0.1.0", "0.0.1", true}, {"0.0.1", "0.0.1", false}, {"0.0.10", "0.0.9", true}, {"1.0.0", "0.99.99", true}, {"0.0.1", "0.1.0", false}, {"1.0", "0.0.1", false}, {"x.y.z", "0.0.1", false},
		// Someone on a pre-release hears of its release, and of nothing that is itself one.
		{"0.1.0", "0.1.0-beta.1", true}, {"0.1.0-beta.2", "0.1.0-beta.1", false}, {"0.2.0-rc.1", "0.1.0", false},
		{"0.1.0", "0.1.0", false}, {"0.0.9", "0.1.0-beta.1", false}, {"0.1.1", "0.1.0-beta.1", true}, {"0.1.0-", "0.0.1", false}} {
		if got := newerVersion(c.a, c.b); got != c.newer {
			t.Errorf("newerVersion(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

// SEC-009, OQ-015: nothing is checked until the user turns the check on;
// then the app asks once a day, without saying anything about itself, and
// reports a newer release; local-only mode stops it.
func TestUpdateCheck(t *testing.T) {
	var hits atomic.Int32
	var ua atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		ua.Store(r.Header.Get("User-Agent") + "|" + r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"tag_name":"v0.2.0","html_url":"https://github.com/FinanceNik/Freeloop/releases/tag/v0.2.0","draft":false,"prerelease":false}`)
	}))
	defer srv.Close()
	e := openTest(t, t.TempDir())
	defer e.Close()
	e.releaseAPI = srv.URL

	if info := e.UpdateStatus(); info.Enabled || info.Newer || info.Current != AppVersion {
		t.Errorf("off: %+v", info)
	}
	time.Sleep(100 * time.Millisecond)
	if hits.Load() != 0 {
		t.Fatal("the app checked for updates while the check was off")
	}

	cfg := e.Config()
	cfg.Updates.Check = true
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	e.UpdateStatus()
	deadline := time.Now().Add(5 * time.Second)
	for !e.UpdateStatus().Newer && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	info := e.UpdateStatus()
	if !info.Newer || info.Latest != "0.2.0" || info.URL != "https://github.com/FinanceNik/Freeloop/releases/tag/v0.2.0" {
		t.Fatalf("on: %+v", info)
	}
	if got := ua.Load().(string); got != "Freeloop-update-check|" {
		t.Errorf("the request said %q about the app", got)
	}
	// Within the day, no second request.
	e.UpdateStatus()
	time.Sleep(100 * time.Millisecond)
	if n := hits.Load(); n != 1 {
		t.Errorf("%d checks within a day", n)
	}

	netguard.SetLocalOnly(true)
	defer netguard.SetLocalOnly(false)
	if _, err := e.CheckForUpdate(context.Background()); err == nil || hits.Load() != 1 {
		t.Errorf("local-only: %v, %d requests", err, hits.Load())
	}
}

// A failing or odd answer leaves the app as it was, with the reason in
// Settings.
func TestUpdateCheckFailures(t *testing.T) {
	for body, want := range map[string]string{
		"404": "no release yet", `{"tag_name":"nightly","html_url":"https://github.com/x"}`: "not a version",
		`{"tag_name":"v9.0.0","html_url":"file:///etc/passwd"}`: "",
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if body == "404" {
				http.NotFound(w, r)
				return
			}
			io.WriteString(w, body)
		}))
		e := openTest(t, t.TempDir())
		e.releaseAPI = srv.URL
		info, err := e.CheckForUpdate(context.Background())
		// A failure waits a day like a success.
		if want != "" && time.Since(e.updateInfo().Checked) > time.Minute {
			t.Errorf("%s: the failed check was not recorded", body)
		}
		switch {
		case want == "" && (err != nil || info.URL != "https://github.com/FinanceNik/Freeloop/releases"):
			t.Errorf("%s: %+v, %v", body, info, err)
		case want != "" && (err == nil || !strings.Contains(err.Error(), want) || info.Newer || !strings.Contains(e.UpdateStatus().Error, want)):
			t.Errorf("%s: %+v, %v", body, info, err)
		}
		e.Close()
		srv.Close()
	}
}
