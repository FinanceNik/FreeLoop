package environment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// GUI-050: a download is kept only when it is the pinned file, and a failed one
// leaves nothing behind.
func TestSaveDownloadKeepsOnlyThePinnedFile(t *testing.T) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte("speech"), 100_000)
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	path := filepath.Join(dir, "voice", "model.bin")

	var last int64
	if err := SaveDownload(path, bytes.NewReader(data), int64(len(data)), sha, func(n int64) { last = n }); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, data) || last != int64(len(data)) {
		t.Fatalf("kept %d bytes, progress %d", len(got), last)
	}

	for name, try := range map[string]struct {
		data []byte
		size int64
	}{
		"another file":      {bytes.Repeat([]byte("x"), len(data)), int64(len(data))},
		"cut short":         {data[:1000], int64(len(data))},
		"larger than known": {append(append([]byte{}, data...), 'x'), int64(len(data))},
	} {
		other := filepath.Join(dir, "other.bin")
		err := SaveDownload(other, bytes.NewReader(try.data), try.size, sha, nil)
		if !errors.Is(err, ErrNotPinned) {
			t.Errorf("%s: %v", name, err)
		}
		for _, p := range []string{other, other + ".part"} {
			if _, err := os.Stat(p); err == nil {
				t.Errorf("%s: %s was left", name, p)
			}
		}
	}
}

func TestInstallFolderStaysInsideIt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "whisper-1.9.5")
	files := []ToolFile{{Name: "whisper-cli", Data: []byte("program")}, {Name: "libwhisper.so.1.9.5", Data: []byte("lib")}}
	if runtime.GOOS != "windows" {
		files = append(files, ToolFile{Name: "libwhisper.so.1", Link: "libwhisper.so.1.9.5"})
	}
	if err := InstallFolder(dir, files); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, files[len(files)-1].Name)); err != nil || len(b) == 0 {
		t.Fatalf("read through the link: %q %v", b, err)
	}
	// Installing again replaces the folder whole.
	if err := InstallFolder(dir, files[:1]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "libwhisper.so.1.9.5")); err == nil {
		t.Error("an old file outlived a new install")
	}
	for _, bad := range [][]ToolFile{
		{{Name: "../escape", Data: []byte("x")}},
		{{Name: `..\escape`, Data: []byte("x")}},
		{{Name: "a/b", Data: []byte("x")}},
		{{Name: "link", Link: "/etc/passwd"}},
		{{Name: "link", Link: "../outside"}},
		{{Name: "link", Link: "missing"}},
	} {
		if err := InstallFolder(dir, bad); err == nil {
			t.Errorf("installed %+v", bad)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "whisper-cli")); err != nil {
		t.Error("a refused install removed what was there")
	}
}
