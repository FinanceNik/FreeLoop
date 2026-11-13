package windows

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Freeloop-0.2.0-windows-amd64-portable.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, body := range files {
		w, _ := z.Create(name)
		w.Write([]byte(body))
	}
	z.Close()
	f.Close()
	return path
}

// SET-008: the exe is replaced by the one in the release's zip, the old one
// moved aside, and a zip without the program changes nothing.
func TestApplyUpdate(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "Freeloop.exe")
	os.WriteFile(exe, []byte("old"), 0o755)
	os.WriteFile(filepath.Join(dir, "LICENSE.txt"), []byte("old license"), 0o644)

	if _, err := applyUpdate(exe, writeZip(t, map[string]string{"Freeloop/LICENSE.txt": "x"})); err == nil {
		t.Error("a zip without the program was installed")
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Fatalf("after a failure the program holds %q", b)
	}

	relaunch, err := applyUpdate(exe, writeZip(t, map[string]string{
		"Freeloop/Freeloop.exe": "new", "Freeloop/LICENSE.txt": "new license", "Freeloop/THIRD_PARTY_NOTICES.txt": "notices",
		// Nothing outside the app's folder is written, whatever a zip names.
		"../../evil.txt": "x",
	}))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"Freeloop.exe": "new", "Freeloop.exe.old": "old", "LICENSE.txt": "new license", "THIRD_PARTY_NOTICES.txt": "notices"} {
		if b, _ := os.ReadFile(filepath.Join(dir, name)); string(b) != want {
			t.Errorf("%s holds %q", name, b)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 4 {
		t.Errorf("the folder holds %v", entries)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(dir)), "evil.txt")); err == nil {
		t.Error("a file was written outside the app's folder")
	}
	if len(relaunch) != 2 || relaunch[0] != exe || relaunch[1] != "--after-update" {
		t.Errorf("relaunch = %v", relaunch)
	}
}
