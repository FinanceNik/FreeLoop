package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// GUI-013: the tree lists folders first and leaves .git out; the viewer
// shows text, says when a file is binary or cut short, and refuses paths
// outside the session's folder.
func TestFileTree(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src", "lib"), 0o755)
	os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	os.WriteFile(filepath.Join(root, "b.txt"), []byte("bee\n"), 0o644)
	os.WriteFile(filepath.Join(root, "A.md"), []byte("# A\n"), 0o644)
	os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(root, "logo.png"), []byte{0x89, 'P', 'N', 'G', 0, 0, 1}, 0o644)
	os.WriteFile(filepath.Join(root, "big.log"), []byte(strings.Repeat("x", maxViewBytes+10)), 0o644)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: DemoProviderID})
	if err != nil {
		t.Fatal(err)
	}
	list, err := e.ListFolder(ctx, ses.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range list {
		names = append(names, f.Name)
	}
	if got := strings.Join(names, ","); got != "src,A.md,b.txt,big.log,logo.png" {
		t.Errorf("root = %s", got)
	}
	sub, _ := e.ListFolder(ctx, ses.ID, "src")
	if len(sub) != 2 || sub[0].Path != "src/lib" || !sub[0].IsDir || sub[1].Path != "src/main.go" {
		t.Errorf("src = %+v", sub)
	}
	if v, err := e.ViewFile(ctx, ses.ID, "src/main.go"); err != nil || v.Text != "package main\n" || v.Binary {
		t.Errorf("main.go = %+v, %v", v, err)
	}
	if v, _ := e.ViewFile(ctx, ses.ID, "logo.png"); !v.Binary || v.Text != "" {
		t.Errorf("logo.png = %+v", v)
	}
	if v, _ := e.ViewFile(ctx, ses.ID, "big.log"); !v.Truncated || len(v.Text) != maxViewBytes {
		t.Errorf("big.log truncated %v, %d bytes", v.Truncated, len(v.Text))
	}
	for _, bad := range []string{"../outside.txt", "/../../etc/passwd"} {
		if _, err := e.ViewFile(ctx, ses.ID, bad); err == nil {
			t.Errorf("%s was read", bad)
		}
	}
	if p, err := e.FileOnHost(ctx, ses.ID, "A.md"); err != nil || !strings.HasSuffix(p, "A.md") {
		t.Errorf("FileOnHost = %s, %v", p, err)
	}
}
