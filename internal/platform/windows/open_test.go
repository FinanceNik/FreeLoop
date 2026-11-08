//go:build windows

package windows

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Nothing is opened by its type but a PDF: a file to edit goes to Notepad, anything
// else shows its folder.
func TestOpeners(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a.txt", "run.bat", "doc.pdf", "fake.pdf.exe"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.exe"), 0o755); err != nil {
		t.Fatal(err)
	}
	open := func(p string) []string { return shellOpen(p) }
	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{"a text file in Notepad", EditorArgv(filepath.Join(dir, "a.txt")), []string{system32("notepad.exe"), filepath.Join(dir, "a.txt")}},
		{"a script in Notepad, never run", EditorArgv(filepath.Join(dir, "run.bat")), []string{system32("notepad.exe"), filepath.Join(dir, "run.bat")}},
		{"a missing file shows its folder", EditorArgv(filepath.Join(dir, "gone.txt")), open(dir)},
		{"a PDF", PDFArgv(filepath.Join(dir, "doc.pdf")), open(filepath.Join(dir, "doc.pdf"))},
		{"a program named like a PDF", PDFArgv(filepath.Join(dir, "fake.pdf.exe")), open(dir)},
		{"a folder named like a program", FolderArgv(filepath.Join(dir, "sub.exe")), open(filepath.Join(dir, "sub.exe"))},
		{"a file shows its folder", FolderArgv(filepath.Join(dir, "run.bat")), open(dir)},
	}
	for _, c := range cases {
		if !slices.Equal(c.got, c.want) {
			t.Errorf("%s: %q, want %q", c.name, c.got, c.want)
		}
	}
}
