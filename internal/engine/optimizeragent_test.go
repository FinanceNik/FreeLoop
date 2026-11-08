package engine

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// tarGz packs files; a value starting with -> is a symbolic link.
func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		h := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if target, ok := strings.CutPrefix(body, "->"); ok {
			h = &tar.Header{Name: name, Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: target}
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			tw.Write([]byte(body))
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestProgramFromArchive(t *testing.T) {
	got, err := programFromArchive(tarGz(t, map[string]string{"./rtk": "the program", "README.md": "x"}), "rtk")
	if err != nil || string(got) != "the program" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := programFromArchive(tarGz(t, map[string]string{"other": "x"}), "rtk"); err == nil {
		t.Error("an archive without the program was accepted")
	}
	if _, err := programFromArchive(tarGz(t, map[string]string{"rtk": "->/bin/sh"}), "rtk"); err == nil {
		t.Error("a link was taken for the program")
	}
	if _, err := programFromArchive([]byte("not an archive"), "rtk"); err == nil {
		t.Error("garbage was accepted")
	}
}

// The download is kept only when it is the build the app knows, and goes only
// into the app's own folder.
func TestInstallOptimizerChecksTheDownload(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	// On Windows the archive holds the program with its .exe.
	archive := tarGz(t, map[string]string{exeName(goruntime.GOOS, "freeloop-test-tool"): "#!/bin/sh\necho hi\n"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(archive) }))
	defer srv.Close()
	sum := sha256.Sum256(archive)
	o := config.Optimizer{ID: "dl", Name: "Dl", Program: "freeloop-test-tool", Download: &config.OptimizerDownload{
		Version: "1.0", Assets: map[string]config.OptimizerAsset{config.AssetKey(): {URL: srv.URL, SHA256: strings.Repeat("0", 64)}},
	}}
	e.optimizers = []config.Optimizer{o}
	if _, err := e.InstallOptimizer(ctx, "dl"); err == nil || !strings.Contains(err.Error(), "not the build") {
		t.Errorf("a wrong checksum: %v", err)
	}
	program := e.downloadedProgram(o)
	if _, err := os.Stat(program); err == nil {
		t.Fatal("a download with a wrong checksum was kept")
	}

	o.Download.Assets[config.AssetKey()] = config.OptimizerAsset{URL: srv.URL, SHA256: hex.EncodeToString(sum[:])}
	if _, err := e.InstallOptimizer(ctx, "dl"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(program, e.appCache("tools")) {
		t.Errorf("installed at %s", program)
	}
	// Windows has no execute bit to check.
	info, err := os.Stat(program)
	if err != nil || goruntime.GOOS != "windows" && info.Mode()&0o100 == 0 {
		t.Fatalf("installed program: %+v, %v", info, err)
	}
	env := mustLocal(t, e)
	if got := e.findProgram(ctx, env, o); got != program {
		t.Errorf("findProgram = %q", got)
	}
	if err := e.RemoveOptimizer(ctx, "dl"); err != nil {
		t.Fatal(err)
	}
	if got := e.findProgram(ctx, env, o); got != "" {
		t.Errorf("after removing, findProgram = %q", got)
	}

	// One with nothing pinned is not downloaded.
	e.optimizers = []config.Optimizer{{ID: "typed", Name: "Typed", Program: "x", Install: "brew install x"}}
	if _, err := e.InstallOptimizer(ctx, "typed"); err == nil {
		t.Error("installed an optimizer with no pinned download")
	}
	if err := e.RemoveOptimizer(ctx, "typed"); err == nil {
		t.Error("removed what the app did not install")
	}
}

func TestAgentsThatTakeAnOptimizer(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	if e.agentTakesOptimizer("claude-code", config.Environment{}) {
		t.Error("available with no program for the hook to run")
	}
	e.hookProgram = "/app/freeloop"
	if !e.agentTakesOptimizer("claude-code", config.Environment{}) {
		t.Error("Claude Code cannot take one")
	}
	if e.agentTakesOptimizer("claude-code", config.Environment{Kind: "docker"}) {
		t.Error("available in a container, which cannot reach the socket")
	}
	if e.agentTakesOptimizer("codex", config.Environment{}) {
		t.Error("available for an agent with no hook")
	}
}

// The plugin's hook is the only thing written for the agent, and only where it can
// work; it names this session.
func TestPluginCarriesTheOptimizerHook(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	e.hookProgram = "/app/free loop"
	dir, err := e.skillPlugin("ses_1", "claude-code", false, session.Project{Root: t.TempDir()}, config.Profile{})
	if err != nil || dir == "" {
		t.Fatalf("plugin = %q, %v", dir, err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type, Command string
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(b, &file); err != nil {
		t.Fatal(err)
	}
	pre := file.Hooks["PreToolUse"]
	if len(pre) != 1 || pre[0].Matcher != "Bash" || len(pre[0].Hooks) != 1 {
		t.Fatalf("hooks = %s", b)
	}
	command := pre[0].Hooks[0].Command
	if !strings.HasPrefix(command, `'/app/free loop' optimizer-hook `) || !strings.HasSuffix(command, "'ses_1'") {
		t.Errorf("command = %s", command)
	}
	if dir, err := e.skillPlugin("ses_2", "claude-code", true, session.Project{Root: t.TempDir()}, config.Profile{}); err != nil || dir != "" {
		t.Errorf("in a container: %q, %v", dir, err)
	}
}

// The hook's answer finds the program by itself, and what the agent then asks about
// maps back to what the model wrote.
func TestRewriteForAgent(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	fakeOptimizer(t, e)
	e.hookProgram = "/app/freeloop"
	root := t.TempDir()
	sid := scriptedSession(t, e, root)
	if got, err := e.RewriteForAgent(ctx, sid, "git status"); got != "" || err != nil {
		t.Errorf("with no running agent: %q, %v", got, err)
	}
	if err := e.SetOptimizer(ctx, sid, "fake"); err != nil {
		t.Fatal(err)
	}
	so := &sessionOptimizer{e: e, sid: sid, env: mustLocal(t, e), agent: true}
	made := so.forAgent(ctx, "git status")
	if !strings.HasPrefix(made, "export PATH=") || !strings.HasSuffix(made, "; fake-optimizer git status") || !strings.Contains(made, "FAKE_OPTIMIZER_TELEMETRY=") {
		t.Fatalf("made = %q", made)
	}
	original, via, ok := so.Original(made)
	if !ok || original != "git status" || via != "Fake" {
		t.Errorf("Original = %q, %q, %v", original, via, ok)
	}
	if _, _, ok := so.Original("git status"); ok {
		t.Error("a command nothing rewrote maps to something")
	}
	for _, command := range []string{"echo plain", "git status | head", "echo evil"} {
		if got := so.forAgent(ctx, command); got != "" {
			t.Errorf("%q was rewritten: %q", command, got)
		}
	}
	// What it remembers is bounded, and the newest stays.
	so.made = map[string]madeCommand{}
	for i := range maxMade {
		so.made[strings.Repeat("x", i+1)] = madeCommand{}
	}
	made = so.forAgent(ctx, "git status")
	if _, _, ok := so.Original(made); !ok || len(so.made) > maxMade {
		t.Errorf("%d remembered, newest found: %v", len(so.made), ok)
	}
	if err := e.SetOptimizer(ctx, sid, ""); err != nil {
		t.Fatal(err)
	}
	if got := so.forAgent(ctx, "git status"); got != "" {
		t.Errorf("rewritten while off: %q", got)
	}
}

func TestOwnHookWarning(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	o := config.Optimizer{ID: "rtk", Name: "RTK", Program: "rtk"}
	agent := session.Session{HarnessID: "claude-code"}
	if got := e.ownHookWarning(agent, o); got != "" {
		t.Errorf("no settings: %q", got)
	}
	write := func(command string) {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"hooks": map[string]any{"PreToolUse": []any{map[string]any{
			"matcher": "Bash", "hooks": []any{map[string]any{"type": "command", "command": command}},
		}}}})
		os.MkdirAll(filepath.Join(e.home, ".claude"), 0o755)
		if err := os.WriteFile(filepath.Join(e.home, ".claude", "settings.json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for command, want := range map[string]bool{
		"rtk hook claude":                true,
		"/opt/bin/rtk hook claude":       true,
		"~/.claude/hooks/rtk-rewrite.sh": true,
		"my-rtk-thing":                   false,
		"artk hook":                      false,
		"/usr/local/bin/lint --fix":      false,
	} {
		write(command)
		if got := e.ownHookWarning(agent, o) != ""; got != want {
			t.Errorf("%q: warned = %v", command, got)
		}
	}
	write("rtk hook claude")
	if got := e.ownHookWarning(session.Session{HarnessID: "builtin"}, o); got != "" {
		t.Errorf("for the built-in agent: %q", got)
	}
}
