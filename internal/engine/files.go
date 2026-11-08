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

// maxPDFBytes is the largest PDF the viewer takes. It goes to the page whole.
const maxPDFBytes = 32 << 20

// PDFFile is a PDF's bytes for the viewer (GUI-035); they travel as base64.
type PDFFile struct {
	Path string `json:"path"`
	Data []byte `json:"data"`
}

// ViewPDF reads a PDF of a session's folder for the viewer, through the same environment
// as any file. Only a file that says it is one: the page hands these bytes to WebKit.
func (e *Engine) ViewPDF(ctx context.Context, sid, rel string) (PDFFile, error) {
	rel, err := cleanRel(rel)
	if err != nil || rel == "" {
		return PDFFile{}, errors.New("choose a file")
	}
	env, err := e.sessionEnv(ctx, sid)
	if err != nil {
		return PDFFile{}, err
	}
	abs, err := inSessionFolder(env, rel)
	if err != nil {
		return PDFFile{}, err
	}
	fi, err := env.FS().Stat(abs)
	if err != nil {
		return PDFFile{}, err
	}
	if fi.IsDir {
		return PDFFile{}, errors.New("that is a folder")
	}
	if fi.Size > maxPDFBytes {
		return PDFFile{}, fmt.Errorf("this PDF is %d MB, and only up to %d MB is shown here; open it in Preview", fi.Size>>20, maxPDFBytes>>20)
	}
	b, err := env.FS().ReadFile(abs)
	if err != nil {
		return PDFFile{}, err
	}
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		return PDFFile{}, errors.New("this file is named as a PDF but is not one")
	}
	return PDFFile{Path: rel, Data: b}, nil
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

// inSessionFolder resolves rel, following links, and refuses it when that leads out
// of the session's folder: a repository's link to a file elsewhere on this Mac is not
// the project's to show.
func inSessionFolder(env environment.Environment, rel string) (string, error) {
	abs := env.Resolve(rel)
	if !env.Contains(abs) {
		return "", errors.New("that leads outside the session's folder")
	}
	return abs, nil
}

// cleanRel checks a path relative to the session's folder.
func cleanRel(rel string) (string, error) {
	// A backslash is a separator on Windows, and filepath.ToSlash makes it one.
	rel = filepath.ToSlash(rel)
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
	dir, err := inSessionFolder(env, cmp.Or(rel, "."))
	if err != nil {
		return nil, err
	}
	infos, err := env.FS().ReadDir(dir)
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
	abs, err := inSessionFolder(env, rel)
	if err != nil {
		return FileView{}, err
	}
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
	abs, err := inSessionFolder(env, rel)
	if err != nil {
		return "", err
	}
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
