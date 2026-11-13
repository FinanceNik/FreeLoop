package engine

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	goruntime "runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/netguard"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/release"
)

// fakeRelease is a stand-in for GitHub holding one release.
type fakeRelease struct {
	t       *testing.T
	srv     *httptest.Server
	version string
	files   map[string][]byte
	// sizes replaces what the release says a file's size is.
	sizes map[string]int64
	// elsewhere makes the release name a file at another address.
	elsewhere string
	hits      atomic.Int32
}

func newFakeRelease(t *testing.T, version string) *fakeRelease {
	f := &fakeRelease{t: t, version: version, files: map[string][]byte{}, sizes: map[string]int64{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		if name, ok := strings.CutPrefix(r.URL.Path, "/download/"); ok {
			b, ok := f.files[name]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Write(b)
			return
		}
		type asset struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		}
		var assets []asset
		for name, b := range f.files {
			a := asset{Name: name, URL: f.srv.URL + "/download/" + name, Size: int64(len(b))}
			if n, ok := f.sizes[name]; ok {
				a.Size = n
			}
			if f.elsewhere != "" && name != release.SumsName && name != release.SigName {
				a.URL = f.elsewhere + "/" + name
			}
			assets = append(assets, a)
		}
		json.NewEncoder(w).Encode(map[string]any{"tag_name": "v" + f.version, "html_url": "https://github.com/FinanceNik/Freeloop/releases/tag/v" + f.version, "assets": assets})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// sign writes SHA256SUMS over the files and signs it as version.
func (f *fakeRelease) sign(key ed25519.PrivateKey, version string) {
	delete(f.files, release.SumsName)
	delete(f.files, release.SigName)
	var sums strings.Builder
	for name, b := range f.files {
		sum := sha256.Sum256(b)
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	f.files[release.SumsName] = []byte(sums.String())
	f.files[release.SigName] = []byte(release.Sign(key, version, []byte(sums.String())))
}

func updateKey(seed string) (ed25519.PrivateKey, string) {
	key := ed25519.NewKeyFromSeed([]byte(strings.Repeat(seed, ed25519.SeedSize)))
	return key, base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
}

// updatable gives the engine a platform that takes any release file and records
// what it was asked to put in place.
func updatable(e *Engine, pub string, f *fakeRelease) (applied *[]string) {
	applied = &[]string{}
	e.releaseAPI = f.srv.URL
	e.releaseKeys = []string{pub}
	e.updateKindOf = func() (string, error) { return release.KindDMG, nil }
	e.applyUpdate = func(_ context.Context, file, version, scratch string, run UpdateRunner) ([]string, error) {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		// The runner runs the system's programs; Windows has no /bin/echo.
		if goruntime.GOOS != "windows" {
			out, err := run(context.Background(), []string{"/bin/echo", "ran"})
			if err != nil || strings.TrimSpace(out) != "ran" {
				return nil, fmt.Errorf("the runner: %q, %v", out, err)
			}
		}
		*applied = append(*applied, version+":"+string(b))
		return []string{"/new/Freeloop", "--after-update"}, nil
	}
	return applied
}

func waitUpdate(t *testing.T, e *Engine) UpdateInfo {
	t.Helper()
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if info := e.UpdateStatus(); info.Step != "downloading" {
			return info
		}
	}
	t.Fatal("the download did not end")
	return UpdateInfo{}
}

// SET-008, SEC-021: the app downloads a release only when asked, keeps its file
// only when the project signed the release under that version and the file is
// the one listed, and hands that file to the platform to put in place.
func TestUpdateInPlace(t *testing.T) {
	ctx := context.Background()
	key, pub := updateKey("k")
	f := newFakeRelease(t, "9.0.0")
	f.files["Freeloop.dmg"] = []byte("the new app")
	f.sign(key, "9.0.0")
	e := openTest(t, t.TempDir())
	defer e.Close()
	applied := updatable(e, pub, f)

	// Knowing of a release downloads none of it.
	if _, err := e.CheckForUpdate(ctx); err != nil {
		t.Fatal(err)
	}
	if info := e.UpdateStatus(); !info.Newer || !info.CanInstall || info.Step != "" || f.hits.Load() != 1 {
		t.Fatalf("after the check: %+v, %d requests", info, f.hits.Load())
	}
	if _, err := e.InstallUpdate(ctx, false); err == nil {
		t.Error("an update that was not downloaded was installed")
	}

	if err := e.DownloadUpdate(); err != nil {
		t.Fatal(err)
	}
	info := waitUpdate(t, e)
	if info.Step != "ready" || info.Ready != "9.0.0" || info.InstallError != "" || info.Received != info.Total || info.Total != int64(len("the new app")) {
		t.Fatalf("after the download: %+v", info)
	}
	relaunch, err := e.InstallUpdate(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(*applied) != 1 || (*applied)[0] != "9.0.0:the new app" || strings.Join(relaunch, " ") != "/new/Freeloop --after-update" {
		t.Errorf("applied %v, relaunch %v", *applied, relaunch)
	}
	if info := e.UpdateStatus(); info.Step != "installing" {
		t.Errorf("after installing: %+v", info)
	}
}

// What is not the project's release, as signed, is never kept or installed.
func TestUpdateRefusals(t *testing.T) {
	key, pub := updateKey("k")
	otherKey, _ := updateKey("o")
	cases := map[string]struct {
		prepare func(f *fakeRelease)
		want    string
	}{
		"signed with another key":                        {func(f *fakeRelease) { f.sign(otherKey, "9.0.0") }, "not signed by the Freeloop project"},
		"an older release's files served as a newer one": {func(f *fakeRelease) { f.sign(key, "8.0.0") }, "not signed by the Freeloop project"},
		"no signature":                                   {func(f *fakeRelease) { f.sign(key, "9.0.0"); delete(f.files, release.SigName) }, "has no SHA256SUMS.sig"},
		"a file changed after signing": {func(f *fakeRelease) {
			f.sign(key, "9.0.0")
			f.files["Freeloop.dmg"] = []byte("another app")
		}, "not the file release 9.0.0 lists"},
		"a file longer than the release says": {func(f *fakeRelease) {
			f.sign(key, "9.0.0")
			f.sizes["Freeloop.dmg"] = 3
		}, "not the file release 9.0.0 lists"},
		"a list changed after signing": {func(f *fakeRelease) {
			f.sign(key, "9.0.0")
			f.files[release.SumsName] = append(f.files[release.SumsName], []byte(strings.Repeat("0", 64)+"  extra\n")...)
		}, "not signed by the Freeloop project"},
		"no file for this computer":   {func(f *fakeRelease) { delete(f.files, "Freeloop.dmg"); f.sign(key, "9.0.0") }, "no file for this computer"},
		"a file at another address":   {func(f *fakeRelease) { f.sign(key, "9.0.0"); f.elsewhere = "https://example.com/releases" }, "not the project's release downloads"},
		"a release that is not newer": {func(f *fakeRelease) { f.version = "0.0.1"; f.sign(key, "0.0.1") }, "not newer"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFakeRelease(t, "9.0.0")
			f.files["Freeloop.dmg"] = []byte("the new app")
			f.files["BUILDINFO.txt"] = []byte("built")
			tc.prepare(f)
			e := openTest(t, t.TempDir())
			defer e.Close()
			applied := updatable(e, pub, f)
			if err := e.DownloadUpdate(); err != nil {
				t.Fatal(err)
			}
			info := waitUpdate(t, e)
			if info.Step != "" || !strings.Contains(info.InstallError, tc.want) {
				t.Errorf("step %q, error %q; want %q", info.Step, info.InstallError, tc.want)
			}
			if _, err := e.InstallUpdate(context.Background(), true); err == nil || len(*applied) != 0 {
				t.Errorf("it was installed: %v, %v", err, *applied)
			}
			if entries, _ := os.ReadDir(e.appCache("updates", "9.0.0")); len(entries) != 0 {
				t.Errorf("kept from a refused download: %v", entries)
			}
		})
	}
}

// A copy that cannot check a release, or cannot be replaced, says why and
// downloads nothing; so does local-only mode. Work is not interrupted unasked.
func TestUpdateNotPossible(t *testing.T) {
	ctx := context.Background()
	key, pub := updateKey("k")
	f := newFakeRelease(t, "9.0.0")
	f.files["Freeloop.dmg"] = []byte("the new app")
	f.sign(key, "9.0.0")
	e := openTest(t, t.TempDir())
	defer e.Close()
	applied := updatable(e, pub, f)
	if _, err := e.CheckForUpdate(ctx); err != nil {
		t.Fatal(err)
	}
	before := f.hits.Load()

	e.releaseKeys = nil
	if info := e.UpdateStatus(); info.CanInstall || !strings.Contains(info.WhyNot, "without a release key") {
		t.Errorf("without a key: %+v", info)
	}
	if err := e.DownloadUpdate(); !errors.Is(err, release.ErrUnsigned) {
		t.Errorf("without a key: %v", err)
	}
	e.releaseKeys = []string{pub}

	e.updateKindOf = func() (string, error) { return "", errors.New("installed as a package") }
	if info := e.UpdateStatus(); info.CanInstall || info.WhyNot != "installed as a package" {
		t.Errorf("a copy that cannot be replaced: %+v", info)
	}
	if err := e.DownloadUpdate(); err == nil {
		t.Error("a copy that cannot be replaced downloaded an update")
	}
	e.updateKindOf = func() (string, error) { return release.KindDMG, nil }

	netguard.SetLocalOnly(true)
	if err := e.DownloadUpdate(); err == nil {
		t.Error("local-only mode downloaded an update")
	}
	netguard.SetLocalOnly(false)
	if f.hits.Load() != before {
		t.Errorf("%d requests were made", f.hits.Load()-before)
	}

	// With a request waiting for an answer, installing asks first.
	if err := e.DownloadUpdate(); err != nil {
		t.Fatal(err)
	}
	if info := waitUpdate(t, e); info.Step != "ready" {
		t.Fatalf("%+v", info)
	}
	withScript(e, "s", scripted.Step{Calls: []scripted.Call{{Name: "write_file", Input: map[string]any{"path": "a.txt", "content": "x"}}}}, scripted.Step{Text: "done"})
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "s", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Prompt(ctx, ses.ID, "go"); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(5 * time.Second); e.CheckQuit(ctx).Empty() && time.Now().Before(end); {
		time.Sleep(10 * time.Millisecond)
	}
	if info := e.UpdateStatus(); info.Busy.Empty() {
		t.Errorf("a waiting request is not said to be interrupted: %+v", info)
	}
	if _, err := e.InstallUpdate(ctx, false); !errors.Is(err, ErrUpdateBusy) || len(*applied) != 0 {
		t.Errorf("installed over waiting work: %v, %v", err, *applied)
	}
	if _, err := e.InstallUpdate(ctx, true); err != nil || len(*applied) != 1 {
		t.Errorf("forced: %v, %v", err, *applied)
	}
}

// The addresses a release file may come from.
func TestReleaseDownload(t *testing.T) {
	e := &Engine{}
	for addr, want := range map[string]bool{
		"https://github.com/FinanceNik/Freeloop/releases/download/v0.2.0/Freeloop.dmg": true,
		"https://github.com/FinanceNik/FreeLoop/releases/download/v0.2.0/Freeloop.dmg": true,
		"https://github.com/Other/Freeloop/releases/download/v0.2.0/Freeloop.dmg":      false,
		"https://github.com.evil.example/FinanceNik/Freeloop/releases/download/x":      false,
		"http://github.com/FinanceNik/Freeloop/releases/download/v0.2.0/Freeloop.dmg":  false,
		"https://example.com/Freeloop.dmg":                                             false,
		"":                                                                             false,
	} {
		if got := e.releaseDownload(addr); got != want {
			t.Errorf("%q: %v", addr, got)
		}
	}
}
