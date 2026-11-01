package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
)

// nestedRepo puts a repository at repo/sub whose own config runs a filter
// that leaves a marker, as an extracted archive or a container could.
func nestedRepo(t *testing.T, repo string) (marker string) {
	t.Helper()
	sub := filepath.Join(repo, "sub")
	os.MkdirAll(sub, 0o755)
	gitIn(t, sub, "init", "-q")
	gitIn(t, sub, "config", "user.name", "t")
	gitIn(t, sub, "config", "user.email", "t@t")
	os.WriteFile(filepath.Join(sub, "f.txt"), []byte("one\n"), 0o644)
	gitIn(t, sub, "add", "f.txt")
	gitIn(t, sub, "commit", "-qm", "one")
	marker = filepath.Join(t.TempDir(), "ran")
	gitIn(t, sub, "config", "filter.x.clean", "touch "+marker+"; cat")
	os.WriteFile(filepath.Join(sub, ".gitattributes"), []byte("* filter=x\n"), 0o644)
	return marker
}

// Review finding: a checkpoint's git add started git in a nested repository,
// whose config ran a program on the Mac, with no trust and no click.
func TestCheckpointsLeaveNestedRepositoriesOut(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	marker := nestedRepo(t, repo)
	e := openTest(t, t.TempDir())
	defer e.Close()
	host, err := e.localEnv(repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.snapshot(ctx, host, "ses_x", "one"); err != nil {
		t.Fatal(err)
	}
	// A store from before this fix holds the repository already.
	if _, err := e.shadowGit(ctx, host, "ses_x", checkpointTimeout, "add", "--", "sub"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "sub", "f.txt"), []byte("two\n"), 0o644)
	if _, err := e.snapshot(ctx, host, "ses_x", "two"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("a snapshot ran the nested repository's filter")
	}
	tree, err := e.shadowGit(ctx, host, "ses_x", checkpointTimeout, "ls-tree", "HEAD")
	if err != nil || strings.Contains(tree, "sub") {
		t.Errorf("the snapshot holds the nested repository: %q, %v", tree, err)
	}
}

// Review finding: the panel's status and diff did the same, and .gitmodules
// can override the config that turns it off.
func TestPanelRunsNothingInANestedRepository(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	marker := nestedRepo(t, repo)
	gitIn(t, repo, "add", "sub")
	os.WriteFile(filepath.Join(repo, ".gitmodules"), []byte("[submodule \"sub\"]\n\tpath = sub\n\turl = ./sub\n\tignore = none\n"), 0o644)
	gitIn(t, repo, "add", ".gitmodules")
	gitIn(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "with sub")
	os.WriteFile(filepath.Join(repo, "sub", "f.txt"), []byte("two\n"), 0o644)
	e := openTest(t, t.TempDir())
	defer e.Close()
	env, err := e.localEnv(repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := changes(ctx, env, ""); err != nil {
		t.Fatal(err)
	}
	gitState(ctx, env, true)
	git(ctx, env, "status", "--porcelain", "--untracked-files=no", "--ignore-submodules=dirty")
	git(ctx, env, "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--ignore-submodules=dirty", "HEAD")
	if _, err := os.Stat(marker); err == nil {
		t.Error("the panel ran the nested repository's filter")
	}
}

// Review findings: config-based hooks and a promisor remote got past the
// keys the panel refuses, and .git/commondir past the read-only .git/config.
func TestPanelRefusesMoreOfARepositorysConfig(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	for _, kv := range [][2]string{
		{"hook.e.command", "touch x"},
		{"extensions.partialClone", "origin"},
		{"remote.origin.promisor", "true"},
		{"protocol.ext.allow", "always"},
	} {
		repo := gitRepo(t)
		env, _ := e.localEnv(repo, nil)
		if why := blocked(ctx, env); why != "" {
			t.Fatalf("a plain repository is blocked: %s", why)
		}
		gitIn(t, repo, "config", kv[0], kv[1])
		if blocked(ctx, env) == "" {
			t.Errorf("%s is not refused", kv[0])
		}
	}

	repo := gitRepo(t)
	env, _ := e.localEnv(repo, nil)
	evil := filepath.Join(repo, ".git", "evil")
	for _, d := range []string{"objects", "refs"} {
		os.MkdirAll(filepath.Join(evil, d), 0o755)
	}
	os.WriteFile(filepath.Join(evil, "config"), []byte("[core]\n\trepositoryformatversion = 0\n"), 0o644)
	os.WriteFile(filepath.Join(repo, ".git", "commondir"), []byte("evil\n"), 0o644)
	if blocked(ctx, env) == "" {
		t.Error("a moved common directory is not refused")
	}
	os.Remove(filepath.Join(repo, ".git", "commondir"))

	// A real linked worktree stays allowed.
	wt := filepath.Join(t.TempDir(), "wt")
	gitIn(t, repo, "worktree", "add", "-q", "-b", "other", wt)
	wtEnv, _ := e.localEnv(wt, nil)
	if why := blocked(ctx, wtEnv); why != "" {
		t.Errorf("a linked worktree is blocked: %s", why)
	}
	gitDir := gitIn(t, wt, "rev-parse", "--git-dir")
	os.WriteFile(filepath.Join(gitDir, "commondir"), []byte("../../evil\n"), 0o644)
	if blocked(ctx, wtEnv) == "" {
		t.Error("a linked worktree's moved common directory is not refused")
	}
}

// Review finding: a URL rewrite or a protocol setting in a repository's own
// config let a push run a program.
func TestPushRefusesRewrittenTransports(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	for _, kv := range [][2]string{
		{"url./tmp/bare.insteadOf", "HEAD:refs/heads/feat"},
		{"protocol.ext.allow", "always"},
		{"core.sshCommand", "touch x"},
	} {
		repo := gitRepo(t)
		env, _ := e.localEnv(repo, nil)
		gitIn(t, repo, "config", kv[0], kv[1])
		if pushBlocked(ctx, env) == "" {
			t.Errorf("%s is not refused", kv[0])
		}
	}
}

// Review finding: the compare page ran git in an untrusted project, where its
// config could still reach a program.
func TestDiffStatNeedsTrust(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	e := openTest(t, t.TempDir())
	defer e.Close()
	os.WriteFile(filepath.Join(repo, "new.txt"), []byte("x\n"), 0o644)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: DemoProviderID})
	if err != nil {
		t.Fatal(err)
	}
	ses.BaseCommit = gitIn(t, repo, "rev-parse", "HEAD")
	if files, _, _ := e.diffStat(ctx, ses); files != 0 {
		t.Errorf("counted %d files in an untrusted project", files)
	}
	e.SetProjectTrust(ctx, repo, true)
	if files, _, _ := e.diffStat(ctx, ses); files != 1 {
		t.Errorf("counted %d files in a trusted project", files)
	}
}

// Review finding: the checks page had no trust gate either.
func TestPullRequestChecksNeedTrust(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, _ := demoSession(t, e, "checks", "ask")
	e.emit(ses.ID, event.PullRequest, event.PullRequestData{URL: "https://github.com/acme/app/pull/7", Title: "t", Base: "main", Head: "f"})
	if _, err := e.PullRequestChecks(ctx, ses.ID); err == nil || !strings.Contains(err.Error(), "Trust") {
		t.Errorf("checks in an untrusted project: %v", err)
	}
}

// Review finding: a reviewer on an agent the app cannot put in plan mode is
// not read-only, whatever the page says.
func TestReviewNeedsAReadOnlyMode(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	e := openTest(t, t.TempDir())
	defer e.Close()
	e.mu.Lock()
	e.presets = append(e.presets, config.HarnessPreset{ID: "noplan", Name: "No plan", Command: "true", Modes: map[string]string{"ask": "default"}})
	e.mu.Unlock()
	if _, err := e.SaveProfile(config.Profile{ID: "noplan", Name: "No plan", Harness: "noplan"}); err != nil {
		t.Fatal(err)
	}
	e.SetProjectTrust(ctx, repo, true)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: DemoProviderID})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed\n"), 0o644)
	if _, err := e.RequestReview(ctx, ReviewRequest{SessionID: ses.ID, ProfileID: "noplan"}); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Errorf("review without a plan mode: %v", err)
	}
}

// Review finding: a deny rule did not hold for an @ mention, so the task tool
// or text in a diff or an issue could attach a file the rule refuses.
func TestMentionsHonourDenyRules(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, root := demoSession(t, e, "", "ask")
	os.WriteFile(filepath.Join(root, "secrets.txt"), []byte("hidden value"), 0o644)
	os.WriteFile(filepath.Join(root, "open.txt"), []byte("open value"), 0o644)
	if err := e.AddRule(permission.Rule{Effect: permission.Deny, Tool: "read_file", Path: "secrets.txt"}); err != nil {
		t.Fatal(err)
	}
	env, err := e.localEnv(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := e.embedMentions(ses.ID, env, "see @secrets.txt and @open.txt")
	if len(p.Resources) != 1 || !strings.Contains(p.Resources[0].Text, "open value") {
		t.Errorf("resources = %+v", p.Resources)
	}
}

// Review finding: a title was cut before redaction, and the piece of a secret
// it kept no longer matched.
func TestCutSecretsAreRedacted(t *testing.T) {
	const secret = "sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOP"
	e := openTest(t, t.TempDir())
	defer e.Close()
	e.remember(secret)
	for _, c := range []struct{ in, want string }{
		{"the whole " + secret + " here", "the whole [REDACTED] here"},
		{"cut at the end " + secret[:30], "cut at the end [REDACTED]"},
		{"…" + secret[len(secret)-30:] + " goes on", "…[REDACTED] goes on"},
		{"too short " + secret[:12], "too short " + secret[:12]},
		{"unrelated text", "unrelated text"},
	} {
		if got := string(e.redact(json.RawMessage(c.in))); got != c.want {
			t.Errorf("redact(%q) = %q", c.in, got)
		}
	}
	ses, _ := demoSession(t, e, "", "ask")
	e.emit(ses.ID, event.SessionInfoUpdate, map[string]string{"title": titleFrom("my key is " + secret)})
	got, _ := e.GetSession(context.Background(), ses.ID)
	if strings.Contains(got.Title, "abcdefgh") {
		t.Errorf("title = %q", got.Title)
	}
	made, err := e.CreateSession(context.Background(), CreateSessionRequest{Title: "key " + secret, ProjectRoot: ses.ProjectRoot, ProviderID: DemoProviderID})
	if err != nil || strings.Contains(made.Title, "abcdefgh") {
		t.Errorf("created title = %q, %v", made.Title, err)
	}
}

// Review finding: a worktree copy followed a repository's symlink out of the
// project, and made private files readable by all.
func TestWorktreeCopyStaysInside(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	base := t.TempDir()
	src, dst := filepath.Join(base, "src"), filepath.Join(base, "dst")
	os.MkdirAll(filepath.Join(src, "dir"), 0o755)
	os.MkdirAll(dst, 0o755)
	outside := filepath.Join(base, "taxes.txt")
	os.WriteFile(outside, []byte("private"), 0o600)
	os.Symlink(outside, filepath.Join(src, ".env"))
	os.Symlink(outside, filepath.Join(src, "dir", "link.txt"))
	os.WriteFile(filepath.Join(src, "dir", "own.txt"), []byte("mine"), 0o600)
	a, _ := e.localEnv(src, nil)
	b, _ := e.localEnv(dst, nil)
	if _, err := copyTree(a, b, ".env", 0); err != errMissing {
		t.Errorf("a link out of the project: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".env")); err == nil {
		t.Error("the link's target was copied")
	}
	n, err := copyTree(a, b, "dir", 0)
	if err != nil || n != 1 {
		t.Errorf("copied %d files, %v", n, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "dir", "link.txt")); err == nil {
		t.Error("a link inside a copied folder was followed out of the project")
	}
	if fi, err := os.Stat(filepath.Join(dst, "dir", "own.txt")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("own.txt: %v, %v", fi, err)
	}
}

// Review finding: the search key followed a redirect to another host.
func TestSearchKeyStaysWithItsHost(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	var leaked string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("X-Key")
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/x", http.StatusFound)
	}))
	defer srv.Close()
	cfg := e.Config()
	cfg.Web.SearchURL, cfg.Web.SearchHeader = srv.URL+"/search?q={query}", "X-Key"
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	if err := e.SetWebSearchKey("sekret-value"); err != nil {
		t.Fatal(err)
	}
	if _, err := (sessionWeb{e: e}).Search(ctx, "x"); err == nil {
		t.Error("the redirect to another host was followed")
	}
	if leaked != "" {
		t.Errorf("the other host received the key %q", leaked)
	}
}

// Review finding: an import kept secrets in plain text when their names did
// not look like one, or when they were command-line arguments.
func TestImportFindsMoreSecrets(t *testing.T) {
	for _, name := range []string{"OPENAI_KEY", "STRIPE_KEY", "KEY", "SENTRY_DSN", "API_TOKEN"} {
		if !secretValue(name, "v") {
			t.Errorf("%s is not taken for a secret", name)
		}
	}
	if !secretValue("DATABASE_URL", "postgres://me:pw@host/db") {
		t.Error("a URL with a password is not taken for a secret")
	}
	for _, name := range []string{"DATABASE_URL", "MONKEY", "PATH", "LOG_LEVEL"} {
		if secretValue(name, "https://example.com/db") {
			t.Errorf("%s is taken for a secret", name)
		}
	}
	for _, args := range [][]string{{"--token", "abc"}, {"-y", "--api-key=abc"}, {"https://me:pw@host/x"}} {
		if secretArgument(args) == "" {
			t.Errorf("%v holds no secret", args)
		}
	}
	for _, args := range [][]string{{"-y", "server"}, {"--token-file", "/path"}, {"--token-env=X"}, {"--token"}, {"--auth", "--verbose"}} {
		if got := secretArgument(args); got != "" {
			t.Errorf("%v: %q", args, got)
		}
	}
}
