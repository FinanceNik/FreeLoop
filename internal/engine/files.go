package engine

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// The file tree and viewer (GUI-013): a read-only browser of a session's
// folder, through its environment like every read, so protected paths stay
// refused. Which files the agent changed comes from the transcript and git,
// in the frontend.

// maxViewBytes caps a file shown in the viewer.
const maxViewBytes = 512 << 10

// FileEntry is one entry of a folder.
type FileEntry struct {
	Name string `json:"name"`
	// Path is relative to the session's folder, with /.
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

// FileView is a file's content for the viewer.
type FileView struct {
	Path      string `json:"path"`
	Text      string `json:"text"`
	Size      int64  `json:"size"`
	Binary    bool   `json:"binary"`
	Truncated bool   `json:"truncated"`
}

// sessionEnv is the environment a session's files are read through: its
// runtime's, or a local one on its folder.
func (e *Engine) sessionEnv(ctx context.Context, sid string) (environment.Environment, error) {
	e.mu.Lock()
	r, ok := e.runtimes[sid]
	e.mu.Unlock()
	if ok {
		return r.env, nil
	}
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	if !environment.IsDir(ses.WorkRoot()) {
		return nil, missingRoot(ses)
	}
	return e.localEnv(ses.WorkRoot(), nil)
}

// cleanRel checks a path relative to the session's folder.
func cleanRel(rel string) (string, error) {
	rel = path.Clean("/" + strings.TrimPrefix(rel, "/"))[1:]
	if strings.HasPrefix(rel, "../") || rel == ".." {
		return "", errors.New("the path is outside the session's folder")
	}
	return rel, nil
}

// ListFolder lists a folder of a session's folder ("" for its root):
// folders first, then files, by name; .git is left out.
func (e *Engine) ListFolder(ctx context.Context, sid, rel string) ([]FileEntry, error) {
	rel, err := cleanRel(rel)
	if err != nil {
		return nil, err
	}
	env, err := e.sessionEnv(ctx, sid)
	if err != nil {
		return nil, err
	}
	infos, err := env.FS().ReadDir(env.Resolve(cmp.Or(rel, ".")))
	if err != nil {
		return nil, err
	}
	out := []FileEntry{}
	for _, fi := range infos {
		if fi.Name == ".git" {
			continue
		}
		out = append(out, FileEntry{Name: fi.Name, Path: path.Join(rel, fi.Name), IsDir: fi.IsDir, Size: fi.Size})
	}
	slices.SortFunc(out, func(a, b FileEntry) int {
		if a.IsDir != b.IsDir {
			if a.IsDir {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out, nil
}

// ViewFile reads a file of a session's folder for the viewer: text up to
// 512 KB, or a note that it is binary.
func (e *Engine) ViewFile(ctx context.Context, sid, rel string) (FileView, error) {
	rel, err := cleanRel(rel)
	if err != nil || rel == "" {
		return FileView{}, errors.New("choose a file")
	}
	env, err := e.sessionEnv(ctx, sid)
	if err != nil {
		return FileView{}, err
	}
	abs := env.Resolve(rel)
	fi, err := env.FS().Stat(abs)
	if err != nil {
		return FileView{}, err
	}
	if fi.IsDir {
		return FileView{}, errors.New("that is a folder")
	}
	v := FileView{Path: rel, Size: fi.Size}
	b, err := env.FS().ReadFile(abs)
	if err != nil {
		return FileView{}, err
	}
	if len(b) > maxViewBytes {
		b, v.Truncated = b[:maxViewBytes], true
	}
	if bytes.IndexByte(b, 0) >= 0 {
		v.Binary = true
		return v, nil
	}
	v.Text = strings.ToValidUTF8(string(b), "�")
	return v, nil
}

// FileOnHost returns the absolute path of a file in a session's folder, to
// open it in another app; the session's folder is on this Mac for every
// environment.
func (e *Engine) FileOnHost(ctx context.Context, sid, rel string) (string, error) {
	rel, err := cleanRel(rel)
	if err != nil || rel == "" {
		return "", errors.New("choose a file")
	}
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return "", err
	}
	env, err := e.localEnv(ses.WorkRoot(), nil)
	if err != nil {
		return "", err
	}
	abs := env.Resolve(rel)
	fi, err := env.FS().Stat(abs)
	if err != nil {
		return "", err
	}
	if fi.IsDir {
		return "", errors.New("choose a file")
	}
	return abs, nil
}

// OpenPDF shows a PDF of a session's folder in the app that only ever shows them.
func (e *Engine) OpenPDF(path string) error {
	if e.pdfArgv == nil {
		return errors.New("this build cannot open PDF files")
	}
	return e.openWith(path, e.pdfArgv)
}

// OpenInEditor opens a file in the user's text editor, which only ever shows it:
// an app chosen by the file's type would run a script behind a harmless name.
func (e *Engine) OpenInEditor(path string) error {
	if e.editorArgv == nil {
		return errors.New("this build cannot open files in an editor")
	}
	return e.openWith(path, e.editorArgv)
}

func (e *Engine) openWith(path string, argv func(string) []string) error {
	env, err := e.localEnv(filepath.Dir(path), nil)
	if err != nil {
		return err
	}
	res, err := env.Exec(context.Background(), environment.ExecSpec{Argv: argv(path), Timeout: 30 * time.Second})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("open %s: %s", path, strings.TrimSpace(res.Output))
	}
	return nil
}
