package sbom

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
)

func TestNotices(t *testing.T) {
	root := t.TempDir()
	write := func(path, text string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("mod/github.com/!azure/go@v1.0.0/LICENSE", "azure license")
	write("mod/example.com/noted@v2.0.0/LICENSE.txt", "noted license")
	write("mod/example.com/noted@v2.0.0/NOTICE", "noted notice")
	write("mod/example.com/noted@v2.0.0/main.go", "package main")
	write("mod/example.com/bare@v1.0.0/go.mod", "module bare")
	write("node/react/LICENSE", "react license")
	write("node/a/node_modules/b/LICENSE.md", "nested license")
	write("node/devonly/LICENSE", "must not appear")

	info := &debug.BuildInfo{Deps: []*debug.Module{
		{Path: "example.com/noted", Version: "v2.0.0"},
		{Path: "github.com/Azure/go", Version: "v1.0.0"},
		{Path: "example.com/bare", Version: "v1.0.0"},
		{Path: "example.com/old", Version: "v1.0.0", Replace: &debug.Module{Path: "example.com/noted", Version: "v2.0.0"}},
	}}
	lock := []byte(`{"packages":{
		"":{"version":"0.0.1"},
		"node_modules/react":{"version":"19.0.0"},
		"node_modules/a/node_modules/b":{"version":"1.0.0"},
		"node_modules/devonly":{"version":"1.0.0","dev":true},
		"node_modules/gone":{"version":"1.0.0"}
	}}`)
	src := NoticeSources{ModCache: filepath.Join(root, "mod"), NodeModules: filepath.Join(root, "node")}
	text, missing, err := Notices(App{Name: "Freeloop", Version: "0.1.0"}, info, lock, src)
	if err != nil {
		t.Fatal(err)
	}
	got := string(text)
	for _, want := range []string{"azure license", "noted license", "noted notice", "react license", "nested license", "github.com/Azure/go v1.0.0"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "must not appear") || strings.Contains(got, "package main") {
		t.Errorf("too much in:\n%s", got)
	}
	if strings.Count(got, "noted license") != 1 {
		t.Errorf("a replaced module is listed twice:\n%s", got)
	}
	if want := []string{"example.com/bare v1.0.0", "gone 1.0.0"}; !slices.Equal(missing, want) {
		t.Errorf("missing = %v, want %v", missing, want)
	}
	again, _, _ := Notices(App{Name: "Freeloop", Version: "0.1.0"}, info, lock, src)
	if string(again) != got {
		t.Error("two runs differ")
	}
}
