//go:build linux

package linux

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("x"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

// opens says argv shows path itself; otherwise it shows the folder.
func opens(argv []string, path string) bool {
	return len(argv) == 2 && filepath.Base(argv[0]) == "xdg-open" && argv[1] == path
}

func TestEditorArgvNeverRunsAFile(t *testing.T) {
	dir := t.TempDir()
	plain := write(t, dir, "notes.txt", 0o644)
	if got := EditorArgv(plain); !opens(got, plain) {
		t.Errorf("a plain file: %v", got)
	}
	for _, name := range []string{"run.desktop", "x.AppImage", "tool.JAR"} {
		p := write(t, dir, name, 0o644)
		if got := EditorArgv(p); !opens(got, dir) {
			t.Errorf("%s: %v", name, got)
		}
	}
	// An executable under a harmless name.
	exe := write(t, dir, "readme.txt.sh", 0o755)
	if got := EditorArgv(exe); !opens(got, dir) {
		t.Errorf("an executable: %v", got)
	}
	// A link with a harmless name to a program.
	link := filepath.Join(dir, "harmless.txt")
	if err := os.Symlink(exe, link); err != nil {
		t.Fatal(err)
	}
	if got := EditorArgv(link); !opens(got, dir) {
		t.Errorf("a link to a program: %v", got)
	}
	// A link to a .desktop file.
	dlink := filepath.Join(dir, "harmless2.txt")
	if err := os.Symlink(filepath.Join(dir, "run.desktop"), dlink); err != nil {
		t.Fatal(err)
	}
	if got := EditorArgv(dlink); !opens(got, dir) {
		t.Errorf("a link to a .desktop file: %v", got)
	}
	// Not a file at all, or not there.
	if got := EditorArgv(dir); !opens(got, filepath.Dir(dir)) {
		t.Errorf("a folder: %v", got)
	}
	if got := EditorArgv(filepath.Join(dir, "missing.txt")); !opens(got, dir) {
		t.Errorf("a missing file: %v", got)
	}
}

func TestPDFArgvOnlyOpensAPlainPDF(t *testing.T) {
	dir := t.TempDir()
	pdf := write(t, dir, "a.PDF", 0o644)
	if got := PDFArgv(pdf); !opens(got, pdf) {
		t.Errorf("a pdf: %v", got)
	}
	for name, mode := range map[string]os.FileMode{"run.pdf": 0o755, "page.html": 0o644, "x.desktop": 0o644} {
		p := write(t, dir, name, mode)
		if got := PDFArgv(p); !opens(got, dir) {
			t.Errorf("%s: %v", name, got)
		}
	}
}

func TestFolderArgvOnlyOpensADirectory(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "project")
	appdir := filepath.Join(dir, "Foo.AppImage")
	for _, d := range []string{proj, appdir} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if got := FolderArgv(proj); !opens(got, proj) {
		t.Errorf("a folder: %v", got)
	}
	if got := FolderArgv(appdir); !opens(got, dir) {
		t.Errorf("a folder named like an app image: %v", got)
	}
	file := write(t, dir, "x.desktop", 0o755)
	if got := FolderArgv(file); !opens(got, dir) {
		t.Errorf("a file: %v", got)
	}
	// A dash is never an option.
	if got := FolderArgv("-n"); got[len(got)-1][0] != '/' {
		t.Errorf("a relative name: %v", got)
	}
}
