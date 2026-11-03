package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// The update check (SEC-009, OQ-015): off unless the user turns it on in
// Settings. When on, the app asks GitHub for the project's latest release
// at most once a day and says when it is newer than the running version;
// it downloads and installs nothing (signed builds and in-app updates wait
// on OQ-009). The request carries no version, ID or other detail of this
// copy of the app, and a failure is silent (NFR-OFF-002).

// AppVersion is this build's version; build/config.yml says the same.
const AppVersion = "0.0.1"

// latestReleaseURL is the GitHub API for the project's latest release.
const latestReleaseURL = "https://api.github.com/repos/FinanceNik/Freeloop/releases/latest"

// updateInterval is how often the check runs.
const updateInterval = 24 * time.Hour

// UpdateInfo is what the app knows about a newer release.
type UpdateInfo struct {
	Enabled bool      `json:"enabled"`
	Current string    `json:"current"`
	Latest  string    `json:"latest,omitempty"`
	URL     string    `json:"url,omitempty"`
	Newer   bool      `json:"newer"`
	Checked time.Time `json:"checked"`
	// Error is the last check's failure, shown in Settings only.
	Error string `json:"error,omitempty"`
}

// UpdateStatus returns what the last check found, starting a check in the
// background when the switch is on and the last one is a day old.
func (e *Engine) UpdateStatus() UpdateInfo {
	info := e.updateInfo()
	if info.Enabled && time.Since(info.Checked) > updateInterval && !netguard.LocalOnly() {
		e.updates.Do(func() {
			e.bg.Go(func() {
				defer e.updates.Reset()
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				e.CheckForUpdate(ctx)
			})
		})
	}
	return info
}

func (e *Engine) updateInfo() UpdateInfo {
	st := config.LoadUpdateState(e.dataDir)
	e.mu.Lock()
	enabled, lastErr := e.cfg.Updates.Check, e.updateErr
	e.mu.Unlock()
	enabled = enabled && e.commandApproved(config.UpdatesFingerprint())
	info := UpdateInfo{Enabled: enabled, Current: AppVersion, Latest: st.Latest, URL: st.URL, Checked: st.CheckedAt, Error: lastErr}
	info.Newer = st.Latest != "" && newerVersion(st.Latest, AppVersion)
	return info
}

// CheckForUpdate asks GitHub for the latest release now; "Check now" in
// Settings works even with the daily check off.
func (e *Engine) CheckForUpdate(ctx context.Context) (UpdateInfo, error) {
	if netguard.LocalOnly() {
		return e.updateInfo(), errors.New("the app is set to connect to nothing but this Mac")
	}
	latest, url, err := fetchLatestRelease(ctx, e.releaseURL())
	e.mu.Lock()
	e.updateErr = ""
	if err != nil {
		e.updateErr = err.Error()
	}
	e.mu.Unlock()
	if err != nil {
		// A failed check waits a day too, keeping what the last good one
		// found; "Check now" asks again at once.
		st := config.LoadUpdateState(e.dataDir)
		st.CheckedAt = time.Now().UTC()
		config.SaveUpdateState(e.dataDir, st)
		return e.updateInfo(), err
	}
	if err := config.SaveUpdateState(e.dataDir, config.UpdateState{CheckedAt: time.Now().UTC(), Latest: latest, URL: url}); err != nil {
		return e.updateInfo(), err
	}
	return e.updateInfo(), nil
}

func (e *Engine) releaseURL() string {
	if e.releaseAPI != "" {
		return e.releaseAPI
	}
	return latestReleaseURL
}

func fetchLatestRelease(ctx context.Context, url string) (version, page string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	// GitHub wants a User-Agent; this one says nothing about this copy.
	req.Header.Set("User-Agent", "Freeloop-update-check")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", "", fmt.Errorf("could not reach GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", "", errors.New("the project has published no release yet")
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("GitHub answered %d", resp.StatusCode)
	}
	var rel struct {
		TagName    string `json:"tag_name"`
		HTMLURL    string `json:"html_url"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return "", "", fmt.Errorf("read the release: %w", err)
	}
	v := strings.TrimPrefix(rel.TagName, "v")
	if _, ok := parseVersion(v); !ok || rel.Draft || rel.Prerelease {
		return "", "", fmt.Errorf("the latest release %q is not a version", rel.TagName)
	}
	// Only a page on GitHub opens from the app.
	if !strings.HasPrefix(rel.HTMLURL, "https://github.com/") {
		rel.HTMLURL = "https://github.com/FinanceNik/Freeloop/releases"
	}
	return v, rel.HTMLURL, nil
}

// parseVersion reads major.minor.patch.
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// newerVersion reports whether a is a later version than b.
func newerVersion(a, b string) bool {
	va, ok1 := parseVersion(a)
	vb, ok2 := parseVersion(b)
	if !ok1 || !ok2 {
		return false
	}
	for i := range va {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}
	return false
}

// onceReset runs f once until Reset.
type onceReset struct {
	mu      sync.Mutex
	running bool
}

func (o *onceReset) Do(f func()) {
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return
	}
	o.running = true
	o.mu.Unlock()
	f()
}

func (o *onceReset) Reset() {
	o.mu.Lock()
	o.running = false
	o.mu.Unlock()
}
