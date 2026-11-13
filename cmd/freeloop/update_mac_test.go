//go:build darwin

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/platform/darwin"
	"github.com/FinanceNik/Freeloop/internal/release"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// macApp writes an ad-hoc signed bundle with the app's identifier that prints says.
func macApp(t *testing.T, path, version, says string) {
	t.Helper()
	macos := filepath.Join(path, "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>io.github.financenik.freeloop</string>
<key>CFBundleShortVersionString</key><string>` + version + `</string>
<key>CFBundleExecutable</key><string>Freeloop</string>
<key>CFBundlePackageType</key><string>APPL</string>
</dict></plist>`
	os.WriteFile(filepath.Join(path, "Contents", "Info.plist"), []byte(plist), 0o644)
	os.WriteFile(filepath.Join(macos, "Freeloop"), []byte("#!/bin/sh\necho "+says+"\n"), 0o755)
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", path).CombinedOutput(); err != nil {
		t.Fatalf("codesign: %v\n%s", err, out)
	}
}

// SET-008, SEC-021, from one end to the other on a Mac: a release on a stand-in
// for GitHub, signed with a key, is downloaded by the engine, checked, and put in
// place of an app bundle by the Mac's own code running the system's programs
// through the engine. The same release signed with another key changes nothing.
func TestUpdateInPlaceOnAMac(t *testing.T) {
	ctx := context.Background()
	bundle := filepath.Join(t.TempDir(), "Freeloop.app")
	macApp(t, bundle, "0.1.0", "old")

	src := filepath.Join(t.TempDir(), "src")
	macApp(t, filepath.Join(src, "Freeloop.app"), "9.0.0", "new")
	dmg := filepath.Join(t.TempDir(), "Freeloop.dmg")
	if out, err := exec.Command("/usr/bin/hdiutil", "create", "-volname", "Freeloop", "-srcfolder", src, "-ov", "-format", "UDZO", "-quiet", dmg).CombinedOutput(); err != nil {
		t.Skipf("hdiutil cannot make a disk image here: %v\n%s", err, out)
	}
	image, err := os.ReadFile(dmg)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(image)
	sums := []byte(hex.EncodeToString(sum[:]) + "  Freeloop.dmg\n")

	key := ed25519.NewKeyFromSeed([]byte(strings.Repeat("k", ed25519.SeedSize)))
	pub := base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
	stranger := ed25519.NewKeyFromSeed([]byte(strings.Repeat("s", ed25519.SeedSize)))
	sig := release.Sign(stranger, "9.0.0", sums)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/download/Freeloop.dmg":
			w.Write(image)
		case "/download/SHA256SUMS":
			w.Write(sums)
		case "/download/SHA256SUMS.sig":
			fmt.Fprint(w, sig)
		default:
			type asset struct {
				Name string `json:"name"`
				URL  string `json:"browser_download_url"`
				Size int64  `json:"size"`
			}
			json.NewEncoder(w).Encode(map[string]any{"tag_name": "v9.0.0", "html_url": "https://github.com/FinanceNik/Freeloop/releases/tag/v9.0.0", "assets": []asset{
				{"Freeloop.dmg", srv.URL + "/download/Freeloop.dmg", int64(len(image))},
				{"SHA256SUMS", srv.URL + "/download/SHA256SUMS", int64(len(sums))},
				{"SHA256SUMS.sig", srv.URL + "/download/SHA256SUMS.sig", int64(len(sig))},
			}})
		}
	}))
	defer srv.Close()

	home := t.TempDir()
	e, err := engine.Open(engine.Options{
		DataDir: t.TempDir(), Secrets: secrets.NewMemory(), Home: home, Dev: true, ReleaseAPI: srv.URL, ReleaseKeys: []string{pub},
		UpdateKind: func() (string, error) { return release.KindDMG, nil },
		ApplyUpdate: func(ctx context.Context, file, version, scratch string, run engine.UpdateRunner) ([]string, error) {
			return darwin.ApplyUpdateTo(ctx, bundle, file, version, scratch, run)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	says := func() string {
		out, err := exec.Command(filepath.Join(bundle, "Contents", "MacOS", "Freeloop")).Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	download := func() engine.UpdateInfo {
		if err := e.DownloadUpdate(); err != nil {
			t.Fatal(err)
		}
		for end := time.Now().Add(30 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
			if info := e.UpdateStatus(); info.Step != "downloading" {
				return info
			}
		}
		t.Fatal("the download did not end")
		return engine.UpdateInfo{}
	}

	// Signed by someone else: nothing is kept and the app stays as it was.
	if info := download(); info.Step != "" || !strings.Contains(info.InstallError, "not signed by the Freeloop project") {
		t.Fatalf("a stranger's release: %+v", info)
	}
	if _, err := e.InstallUpdate(ctx, true); err == nil || says() != "old" {
		t.Fatalf("a stranger's release was installed: %v, the app says %q", err, says())
	}

	// Signed by the project.
	sig = release.Sign(key, "9.0.0", sums)
	if info := download(); info.Step != "ready" || info.Ready != "9.0.0" {
		t.Fatalf("the project's release: %+v", info)
	}
	relaunch, err := e.InstallUpdate(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if says() != "new" {
		t.Errorf("the app in place says %q", says())
	}
	if len(relaunch) != 5 || relaunch[0] != "/usr/bin/open" || relaunch[2] != bundle || relaunch[4] != afterUpdateArg {
		t.Errorf("relaunch = %v", relaunch)
	}
	if out, err := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", bundle).CombinedOutput(); err != nil {
		t.Errorf("the app in place is not validly signed: %s", out)
	}
}
