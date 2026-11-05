package engine

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider"
)

func TestMentionPaths(t *testing.T) {
	got := mentionPaths(`@a.go, see @"docs/my notes.md" and mail me@example.com; again @a.go. (@b/c.ts)` + "\n@d")
	want := []string{"a.go", "docs/my notes.md", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mentionPaths = %q, want %q", got, want)
	}
}

func TestEmbedMentions(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, root := demoSession(t, e, "", "ask")
	os.MkdirAll(filepath.Join(root, "docs"), 0o755)
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package a"), 0o644)
	os.WriteFile(filepath.Join(root, "docs", "my notes.md"), []byte("# notes"), 0o644)
	os.WriteFile(filepath.Join(root, "bin.dat"), []byte{1, 0, 2}, 0o644)
	os.WriteFile(filepath.Join(root, "big.txt"), []byte(strings.Repeat("x", maxMentionBytes+1)), 0o644)
	outside := filepath.Join(filepath.Dir(root), "outside.txt")
	os.WriteFile(outside, []byte("secret"), 0o644)
	env, err := e.localEnv(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	ch, unsub := e.Subscribe()
	defer unsub()

	p := e.embedMentions(ses.ID, env, `fix @a.go using @"docs/my notes.md", not @docs or @missing.go or @../outside.txt or @`+outside+` or @bin.dat or @big.txt`)
	var paths []string
	for _, r := range p.Resources {
		paths = append(paths, r.Path)
	}
	if !reflect.DeepEqual(paths, []string{"a.go", "docs/my notes.md"}) {
		t.Errorf("attached %q", paths)
	}
	if p.Resources[0].Text != "package a" || !strings.HasPrefix(p.Resources[0].URI, "file:///") {
		t.Errorf("resource = %+v", p.Resources[0])
	}
	ev := waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.Notice })
	msg := string(ev.Data)
	if !strings.Contains(msg, "@bin.dat (binary file)") || !strings.Contains(msg, "@big.txt (larger than 256 KB)") || strings.Contains(msg, "outside") || strings.Contains(msg, "missing") {
		t.Errorf("notice = %s", msg)
	}

	files, err := e.ProjectFiles(ctx, ses.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(files, []string{"a.go", "big.txt", "bin.dat", "docs/my notes.md"}) {
		t.Errorf("ProjectFiles = %q", files)
	}
}

func TestContextLimit(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	p, err := e.SaveProvider(SaveProviderRequest{Name: "Work", Type: "openai_compat", Preset: "custom", BaseURL: "http://127.0.0.1:1/v1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	limitOf := func() provider.ModelCaps {
		t.Helper()
		caps, err := e.ModelCapabilities(ctx, p.ID, "gpt", false)
		if err != nil {
			t.Fatal(err)
		}
		return caps
	}
	if c := limitOf(); c.ContextWindow != 0 || c.ContextLimitSet {
		t.Fatalf("before any limit: %+v", c)
	}
	for _, bad := range []int{-1, 999, 100_000_001} {
		if err := e.SetContextLimit(ctx, p.ID, "gpt", bad); err == nil {
			t.Errorf("limit %d was accepted", bad)
		}
	}
	if err := e.SetContextLimit(ctx, p.ID, "gpt", 200_000); err != nil {
		t.Fatal(err)
	}
	if c := limitOf(); c.ContextWindow != 200_000 || !c.ContextLimitSet {
		t.Errorf("after a limit: %+v", c)
	}
	// It is in the file, for the next start.
	cfgs, _, err := config.LoadProviders(e.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfgs[0].Models["gpt"].ContextWindow; got != 200_000 {
		t.Errorf("stored limit %d", got)
	}
	if c, err := e.ModelCapabilities(ctx, p.ID, "other", false); err != nil || c.ContextWindow != 0 {
		t.Errorf("another model took the limit: %+v, %v", c, err)
	}
	// Clearing it leaves nothing stored.
	if err := e.SetContextLimit(ctx, p.ID, "gpt", 0); err != nil {
		t.Fatal(err)
	}
	if c := limitOf(); c.ContextWindow != 0 || c.ContextLimitSet {
		t.Errorf("after clearing: %+v", c)
	}
	cfgs, _, _ = config.LoadProviders(e.dataDir)
	if _, ok := cfgs[0].Models["gpt"]; ok {
		t.Error("a cleared limit left an entry")
	}
}

func TestModelFlags(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	p, err := e.SaveProvider(SaveProviderRequest{Name: "Local", Type: "ollama", BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	on := true
	e.SetToolsOverride(p.ID, "qwen", &on)
	if err := e.SetModelFlags(p.ID, "qwen", true, false); err != nil {
		t.Fatal(err)
	}
	e.SetModelFlags(p.ID, "llama", false, true)
	if err := e.SetModelFlags(p.ID, "x", true, true); err == nil {
		t.Error("favorite and hidden at once was accepted")
	}
	find := func() ProviderInfo {
		for _, i := range e.ListProviders() {
			if i.ID == p.ID {
				return i
			}
		}
		t.Fatal("provider gone")
		return ProviderInfo{}
	}
	got := find()
	if !reflect.DeepEqual(got.Favorites, []string{"qwen"}) || !reflect.DeepEqual(got.Hidden, []string{"llama"}) {
		t.Errorf("favorites %q hidden %q", got.Favorites, got.Hidden)
	}
	// Clearing the flags keeps the tools override; clearing everything
	// drops the entry.
	e.SetModelFlags(p.ID, "qwen", false, false)
	e.SetModelFlags(p.ID, "llama", false, false)
	e.mu.Lock()
	models := e.providers.configs[0].Models
	e.mu.Unlock()
	if len(models) != 1 || models["qwen"].Tools == nil {
		t.Errorf("models = %+v", models)
	}
}
