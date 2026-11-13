package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/netguard"
	"github.com/FinanceNik/Freeloop/internal/release"
)

// Updating in place (SET-008). When the user asks, the app downloads the
// latest release's file for this system from GitHub, keeps it only if the
// project signed that release (SEC-021) and the file is the one the release
// lists, and then puts it in place of the running copy and starts it. Nothing
// is downloaded before the click, and local-only mode downloads nothing.
//
// What "in place" means differs by system, so that part is the platform's
// (Options.UpdateKind, Options.ApplyUpdate): a Mac's app bundle from the disk
// image, a Windows exe from the portable zip, an AppImage. A copy that cannot
// be replaced, one installed from a .deb for one, says why and keeps the link
// to the release's page.

// releaseDownloads is where GitHub serves a release's files from. Only
// addresses under it are fetched: the signature guards what a file is, and
// this guards whom the app talks to (SEC-009).
const releaseDownloads = "https://github.com/FinanceNik/Freeloop/releases/download/"

// maxUpdateSize bounds a release file; the Mac's disk image is about a tenth.
const maxUpdateSize = 2 << 30

// updateDownloadFor bounds a download.
const updateDownloadFor = 30 * time.Minute

// UpdateRunner runs one of the system's own programs for the platform's part
// of an update and returns what it printed.
type UpdateRunner = func(ctx context.Context, argv []string) (string, error)

type selfUpdate struct {
	mu       sync.Mutex
	step     string
	received int64
	total    int64
	err      string
	// file is the downloaded release file of version, once step is "ready".
	file, version string
	cancel        context.CancelFunc
}

// stop ends a download that is under way.
func (u *selfUpdate) stop() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.cancel != nil {
		u.cancel()
	}
}

// selfUpdateInfo fills in what UpdateInfo says about updating in place.
func (e *Engine) selfUpdateInfo(info *UpdateInfo) {
	u := &e.selfUpdate
	u.mu.Lock()
	info.Step, info.Received, info.Total, info.InstallError = u.step, u.received, u.total, u.err
	if u.step == "ready" {
		info.Ready = u.version
	}
	u.mu.Unlock()
	if !info.Newer && info.Step == "" {
		return
	}
	if _, err := e.updateKind(); err != nil {
		info.WhyNot = err.Error()
		return
	}
	info.CanInstall = true
	if info.Step == "ready" {
		info.Busy = e.CheckQuit(context.Background())
	}
}

// updateKind is the kind of release file this copy updates from, or why it
// cannot update in place.
func (e *Engine) updateKind() (string, error) {
	if len(e.releaseKeys) == 0 {
		return "", release.ErrUnsigned
	}
	if e.updateKindOf == nil || e.applyUpdate == nil {
		return "", errors.New("this build cannot replace itself; download the release from its page")
	}
	return e.updateKindOf()
}

// DownloadUpdate starts downloading the latest release for this copy, in the
// background; UpdateStatus says how far it is. It is the user's click: the
// daily check never calls it.
func (e *Engine) DownloadUpdate() error {
	if netguard.LocalOnly() {
		return errors.New("the app is set to connect to nothing but this computer")
	}
	kind, err := e.updateKind()
	if err != nil {
		return err
	}
	u := &e.selfUpdate
	u.mu.Lock()
	if u.step == "downloading" || u.step == "installing" {
		u.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), updateDownloadFor)
	u.step, u.received, u.total, u.err, u.file, u.version, u.cancel = "downloading", 0, 0, "", "", "", cancel
	u.mu.Unlock()
	e.bg.Go(func() {
		defer cancel()
		file, version, err := e.fetchUpdate(ctx, kind)
		u.mu.Lock()
		defer u.mu.Unlock()
		u.cancel = nil
		if err != nil {
			u.step, u.err = "", err.Error()
			if errors.Is(ctx.Err(), context.Canceled) {
				u.err = ""
			}
			return
		}
		u.step, u.file, u.version = "ready", file, version
	})
	return nil
}

// CancelUpdate stops a download and forgets one that is ready.
func (e *Engine) CancelUpdate() {
	u := &e.selfUpdate
	u.mu.Lock()
	if u.step == "installing" {
		u.mu.Unlock()
		return
	}
	if u.cancel != nil {
		u.cancel()
	}
	u.step, u.file, u.version, u.err = "", "", "", ""
	u.mu.Unlock()
}

// fetchUpdate downloads the latest release's file of a kind and returns where
// it is kept, once the release is signed by the project and the file is the
// one it lists.
func (e *Engine) fetchUpdate(ctx context.Context, kind string) (file, version string, err error) {
	rel, err := fetchLatestRelease(ctx, e.releaseURL())
	if err != nil {
		return "", "", err
	}
	if !newerVersion(rel.Version, AppVersion) {
		return "", "", fmt.Errorf("release %s is not newer than this copy (%s)", rel.Version, AppVersion)
	}
	sums, err := e.releaseText(ctx, rel, release.SumsName)
	if err != nil {
		return "", "", err
	}
	sig, err := e.releaseText(ctx, rel, release.SigName)
	if err != nil {
		return "", "", err
	}
	// The version is signed with the list, so an older release's files cannot
	// be served as this one.
	if err := release.Verify(e.releaseKeys, rel.Version, sums, string(sig)); err != nil {
		return "", "", err
	}
	list, err := release.Sums(sums)
	if err != nil {
		return "", "", err
	}
	name, err := release.Asset(kind, rel.Version, goruntime.GOARCH)
	if err != nil {
		return "", "", err
	}
	asset, listed := rel.Assets[name]
	sha := list[name]
	if !listed || sha == "" {
		return "", "", fmt.Errorf("release %s has no file for this computer (%s); download it from the release's page", rel.Version, name)
	}
	if asset.Size <= 0 || asset.Size > maxUpdateSize {
		return "", "", fmt.Errorf("%s is %d bytes, which no release file is", name, asset.Size)
	}
	resp, err := e.releaseGet(ctx, asset.URL)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	u := &e.selfUpdate
	u.mu.Lock()
	u.total = asset.Size
	u.mu.Unlock()
	// One download is kept at a time: an older one is of no use.
	dir := e.appCache("updates")
	if err := environment.RemoveCache(dir); err != nil {
		return "", "", err
	}
	file = filepath.Join(dir, rel.Version, name)
	err = environment.SaveDownload(file, resp.Body, asset.Size, sha, func(n int64) {
		u.mu.Lock()
		u.received = n
		u.mu.Unlock()
	})
	if errors.Is(err, environment.ErrNotPinned) {
		return "", "", fmt.Errorf("the download of %s is not the file release %s lists, so it was not kept", name, rel.Version)
	}
	if err != nil {
		return "", "", err
	}
	return file, rel.Version, nil
}

// releaseText fetches one of a release's small text files.
func (e *Engine) releaseText(ctx context.Context, rel latestRelease, name string) ([]byte, error) {
	asset, ok := rel.Assets[name]
	if !ok {
		return nil, fmt.Errorf("release %s has no %s, so this copy cannot tell it is the project's; download it from the release's page", rel.Version, name)
	}
	resp, err := e.releaseGet(ctx, asset.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", name, err)
	}
	if len(b) > 1<<20 {
		return nil, fmt.Errorf("%s is larger than any release's", name)
	}
	return b, nil
}

// releaseGet fetches a release file, from GitHub's release downloads only.
func (e *Engine) releaseGet(ctx context.Context, addr string) (*http.Response, error) {
	if !e.releaseDownload(addr) {
		return nil, fmt.Errorf("the release names a file at %s, which is not the project's release downloads", addr)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Freeloop-update-check")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach GitHub: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GitHub answered %d for %s", resp.StatusCode, filepath.Base(addr))
	}
	return resp, nil
}

// releaseDownload reports whether addr is one of the project's release
// downloads. GitHub writes the repository's name as its owner spelled it, so
// case does not count. A development build's stand-in for GitHub
// (Options.ReleaseAPI) serves its own.
func (e *Engine) releaseDownload(addr string) bool {
	if len(addr) >= len(releaseDownloads) && strings.EqualFold(addr[:len(releaseDownloads)], releaseDownloads) {
		return true
	}
	if e.releaseAPI == "" {
		return false
	}
	api, err1 := url.Parse(e.releaseAPI)
	got, err2 := url.Parse(addr)
	return err1 == nil && err2 == nil && got.Scheme == api.Scheme && got.Host == api.Host
}

// ErrUpdateBusy says installing now would interrupt work; force installs anyway.
var ErrUpdateBusy = errors.New("sessions are working or waiting for an answer; installing the update would interrupt them")

// InstallUpdate puts the downloaded release in place of the running copy and
// returns the command that starts it. The caller starts that and quits. With
// work that quitting would interrupt it refuses, unless force.
func (e *Engine) InstallUpdate(ctx context.Context, force bool) (relaunch []string, err error) {
	if _, err := e.updateKind(); err != nil {
		return nil, err
	}
	if !force && !e.CheckQuit(ctx).Empty() {
		return nil, ErrUpdateBusy
	}
	u := &e.selfUpdate
	u.mu.Lock()
	if u.step != "ready" {
		u.mu.Unlock()
		return nil, errors.New("no update is downloaded")
	}
	file, version := u.file, u.version
	u.step = "installing"
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		defer u.mu.Unlock()
		if err != nil {
			// The download is kept: what failed may be fixed by the user, a folder
			// that could not be written for one.
			u.step, u.err = "ready", err.Error()
		}
	}()
	scratch := filepath.Dir(file)
	env, err := e.localEnv(scratch, nil)
	if err != nil {
		return nil, err
	}
	run := func(ctx context.Context, argv []string) (string, error) {
		res, err := env.Exec(ctx, environment.ExecSpec{Argv: argv, Timeout: 5 * time.Minute})
		if err != nil {
			return "", err
		}
		if res.ExitCode != 0 {
			return res.Output, fmt.Errorf("%s: %s", filepath.Base(argv[0]), strings.TrimSpace(res.Output))
		}
		return res.Output, nil
	}
	relaunch, err = e.applyUpdate(ctx, file, version, scratch, run)
	if err != nil {
		return nil, fmt.Errorf("install %s: %w", version, err)
	}
	if len(relaunch) == 0 {
		return nil, fmt.Errorf("install %s: the platform named no way to start it", version)
	}
	return relaunch, nil
}
