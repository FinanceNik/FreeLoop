package engine

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
)

// Installing an optimizer from inside the app (HRN-019): the build a built-in one
// names, at a pinned version and checksum, into the app's own caches. Nothing goes
// on the user's PATH or anywhere another program looks, and only sessions that
// choose it use it. It runs only on the user's asking; local-only mode refuses it.

const (
	maxToolDownload = 100 << 20
	toolDownloadFor = 2 * time.Minute
)

// toolsDir holds what the app downloaded. A sandboxed command can read it, to run
// it, and cannot write it.
func (e *Engine) toolsDir() string {
	return filepath.Join(environment.Canonical(e.home), "Library", "Caches", "Freeloop", "tools")
}

// downloadedProgram is where an optimizer's download goes.
func (e *Engine) downloadedProgram(o config.Optimizer) string {
	if o.Download == nil {
		return ""
	}
	return filepath.Join(e.toolsDir(), o.ID+"-"+o.Download.Version, o.Program)
}

// findProgram finds an optimizer's program: on the PATH commands see, else what
// the app downloaded for it.
func (e *Engine) findProgram(ctx context.Context, env environment.Environment, o config.Optimizer) string {
	if p := locateProgram(ctx, env, o.Program); p != "" {
		return p
	}
	if env.Kind() == environment.KindDocker {
		return ""
	}
	if p := e.downloadedProgram(o); p != "" && environment.IsFile(p) {
		return p
	}
	return ""
}

// InstallOptimizer downloads an optimizer's program, checks it and keeps it.
func (e *Engine) InstallOptimizer(ctx context.Context, id string) (OptimizerStatus, error) {
	o, ok := e.findOptimizer(id)
	if !ok {
		return OptimizerStatus{}, fmt.Errorf("no token optimizer %q", id)
	}
	if o.Download == nil {
		return OptimizerStatus{}, fmt.Errorf("Freeloop cannot install %s; install it with: %s", o.Name, o.Install)
	}
	asset, ok := o.Download.Assets[goruntime.GOARCH]
	if !ok {
		return OptimizerStatus{}, fmt.Errorf("%s has no build for this Mac (%s)", o.Name, goruntime.GOARCH)
	}
	ctx, cancel := context.WithTimeout(ctx, toolDownloadFor)
	defer cancel()
	archive, err := fetchTool(ctx, asset.URL)
	if err != nil {
		return OptimizerStatus{}, fmt.Errorf("download %s: %w", o.Name, err)
	}
	sum := sha256.Sum256(archive)
	if hex.EncodeToString(sum[:]) != asset.SHA256 {
		return OptimizerStatus{}, fmt.Errorf("the download of %s is not the build Freeloop knows, so it was not kept", o.Name)
	}
	program, err := programFromArchive(archive, o.Program)
	if err != nil {
		return OptimizerStatus{}, fmt.Errorf("unpack %s: %w", o.Name, err)
	}
	if err := environment.InstallTool(e.downloadedProgram(o), program); err != nil {
		return OptimizerStatus{}, err
	}
	for _, st := range e.ListOptimizers(ctx) {
		if st.ID == id {
			return st, nil
		}
	}
	return OptimizerStatus{}, errors.New("it was installed but is no longer listed")
}

// RemoveOptimizer deletes what the app downloaded for an optimizer.
func (e *Engine) RemoveOptimizer(ctx context.Context, id string) error {
	o, ok := e.findOptimizer(id)
	if !ok || o.Download == nil {
		return fmt.Errorf("Freeloop did not install %q", id)
	}
	return environment.RemoveCache(filepath.Dir(e.downloadedProgram(o)))
}

func fetchTool(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the server answered %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxToolDownload+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxToolDownload {
		return nil, errors.New("it is larger than expected")
	}
	return b, nil
}

// programFromArchive returns the file named program from a .tar.gz.
func programFromArchive(archive []byte, program string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("it does not hold %s", program)
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg && path.Base(strings.TrimPrefix(h.Name, "./")) == program {
			b, err := io.ReadAll(io.LimitReader(tr, maxToolDownload+1))
			if err != nil {
				return nil, err
			}
			if len(b) > maxToolDownload {
				return nil, errors.New("it is larger than expected")
			}
			return b, nil
		}
	}
}
