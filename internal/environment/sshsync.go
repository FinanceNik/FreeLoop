package environment

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"
)

// The sync between the project and a working copy (ENV-008).
//
// It has no rsync in it, on purpose. Receiving from a machine that may be compromised
// would run rsync's receiver here against a hostile sender, which has a history of
// writing anywhere and reading anything (CVE-2022-29154, CVE-2024-12086). What comes
// back is a list of names and a tar stream that this file reads as data:
//   - a name it did not ask for is ignored, and only plain paths are ever asked for;
//   - only regular files are taken: no link, device or other kind is ever made here;
//   - no file gains an executable bit from over there, and an executable one here is
//     not replaced;
//   - sizes and counts are capped, with no compression to expand;
//   - a path through a symlink here is refused, and writes pass the Guard;
//   - settings that this Mac runs or an agent obeys are held back, whatever they say;
//   - the user's own edits win over what comes back;
//   - if the project's ignore rules cannot be read, nothing comes back;
//   - what is replaced or deleted is set aside first, and a mass delete is not applied,
//     in one step or several.
//
// A file of the project a person runs on purpose can still change, a Makefile or a test
// among them: that is what the agent is there for, and it shows in the diff.

const (
	syncTimeout = 5 * time.Minute
	// Caps on what comes back in one sync, and on one file either way.
	maxSyncFile  = 64 << 20
	maxPullTotal = 1 << 30
	maxPullFiles = 20000
	maxListBytes = 32 << 20
	maxListNames = 500_000
	// What comes back over a whole session, and what is kept of what it replaces.
	maxPullSession = 16 << 30
	maxSetAside    = 1 << 30
	// What the sync remembers having told the user.
	maxSaid = 50_000
	// A delete from over there of more than this share of the files, past the count,
	// is not applied. A runaway rm or a hostile one should not empty the project.
	massDeleteCount = 25
	massDeleteShare = 4 // a quarter
)

// synced is what the sync remembers. A file's baseline is its content when both sides
// last agreed, so a change on each side can be told apart.
type synced struct {
	baseline  map[string]string
	stats     map[string]statHash
	said      map[string]bool
	newerFlag string
	// Across the session: files deleted from here, the most files there were, what came
	// back and what was set aside.
	deleted, peakFiles   int
	pulled, setAsideSize int64
	setAsideSeq          int
}

type statHash struct {
	size int64
	mod  time.Time
	hash string
}

func newSynced() synced {
	return synced{baseline: map[string]string{}, stats: map[string]statHash{}, said: map[string]bool{}}
}

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// validSyncPath reports a relative path that means what it says on both sides: plain,
// clean, and with nothing invisible in it that could make it read as another.
func validSyncPath(rel string) bool {
	if rel == "" || len(rel) > 4096 || !utf8.ValidString(rel) || strings.HasPrefix(rel, "/") || strings.Contains(rel, `\`) {
		return false
	}
	for _, r := range rel {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	if path.Clean(rel) != rel {
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		// A leading ~ would mean the home folder to whatever reads the path next.
		if part == "" || part == "." || part == ".." || part == "~" || len(part) > 255 {
			return false
		}
	}
	return true
}

// What crosses is decided by name, at any depth, the way this Mac's file system compares
// names, so .GIT/config is .git/config.
//
// Neither way: settings of tools and agents, which can hold secrets and which what is
// over there has no use for, and installed code. What this Mac would run or an agent
// would obey with no one reading it first must not come back: a Python package in a
// .venv runs at the next python, whatever git status shows.
//
// Back only: what the other machine should have but must not change here. A project's
// AGENTS.md would steer the next local session, and a .gitignore decides what is sent:
// emptied over there, it would send every file the project ignores.
var (
	settingDirs = nameSet(".git", ".freeloop", ".agents", ".claude", ".codex", ".gemini", ".cursor", ".opencode", ".goose", ".windsurf", ".continue", ".zed",
		".vscode", ".idea", ".direnv", ".cargo", ".bundle", ".yarn", ".mise",
		"node_modules", ".venv", "venv", "__pycache__", "site-packages")
	settingFiles = nameSet(".envrc", ".mcp.json", ".yarnrc", ".yarnrc.yml", ".pnpmfile.cjs", ".mise.toml", "mise.toml", ".tool-versions", "opencode.json", ".nvim.lua", ".nvimrc", ".exrc")
	heldDirs     = nameSet(".devcontainer", ".husky", ".githooks", ".github", ".gitlab", ".circleci", ".mvn")
	heldFiles    = nameSet("agents.md", "claude.md", "gemini.md",
		".gitignore", ".ignore", ".gitattributes", ".gitmodules", ".lfsconfig",
		// With objects and refs beside it, a HEAD makes its folder a bare repository, and
		// git run there obeys the config beside it, whose core.fsmonitor is a command.
		"head",
		".devcontainer.json", ".pre-commit-config.yaml", ".gitlab-ci.yml", ".travis.yml", ".bazelrc", "gradle.properties", ".pylintrc", "pylintrc")
	secretDirs = nameSet(".ssh", ".gnupg", ".aws", ".azure", ".gcloud", ".kube", ".docker", ".terraform")
	// Whole names, and how a name starts or ends.
	secretFiles = nameSet(".netrc", ".npmrc", ".pypirc", ".git-credentials", ".pgpass", ".vault-token", ".s3cfg", ".htpasswd", "kubeconfig", "terraform.tfvars")
)

func nameSet(names ...string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[FoldPath(n)] = true
	}
	return set
}

// inAny reports a path that is one of the names or inside one.
func inAny(parts []string, dirs, files map[string]bool) bool {
	for _, part := range parts[:len(parts)-1] {
		if dirs[part] {
			return true
		}
	}
	last := parts[len(parts)-1]
	return dirs[last] || files[last]
}

// settingPath reports a path that crosses neither way. A name of acted is one the
// permission policy knows something outside a session acts on.
func settingPath(rel string, acted map[string]bool) bool {
	parts := strings.Split(FoldPath(rel), "/")
	return inAny(parts, settingDirs, settingFiles) || inAny(parts, acted, acted)
}

// heldPath reports a path that does not come back.
func heldPath(rel string) bool {
	parts := strings.Split(FoldPath(rel), "/")
	return inAny(parts, settingDirs, settingFiles) || inAny(parts, heldDirs, heldFiles)
}

// SyncHeldNames lists the names of files and folders the sync keeps from crossing,
// one way or both, for the permission policy to be checked against (SEC-005).
func SyncHeldNames() []string {
	var out []string
	for _, set := range []map[string]bool{settingDirs, settingFiles, heldDirs, heldFiles} {
		for name := range set {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// SecretPath reports a path, relative to the project, that holds secrets by its name:
// what the sync never sends, and the broker asks about before it is read.
func SecretPath(rel string) bool { return secretPath(rel) }

// secretPath reports a path that never leaves this Mac, tracked or not, and does not
// come back either.
func secretPath(rel string) bool {
	parts := strings.Split(FoldPath(rel), "/")
	if inAny(parts, secretDirs, secretFiles) {
		return true
	}
	name := parts[len(parts)-1]
	switch name {
	case ".env.example", ".env.sample", ".env.template":
		return false
	}
	for _, p := range []string{".env.", "id_rsa", "id_dsa", "id_ecdsa", "id_ed25519", "credentials", "secret", "service-account", "service_account", "serviceaccount"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	if name == ".env" {
		return true
	}
	for _, ext := range []string{".env", ".pem", ".key", ".p12", ".pfx", ".ppk", ".keystore", ".jks", ".tfvars", ".tfvars.json", ".tfstate", ".tfstate.backup", "-key.json", "_key.json"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// keepsHere reports a path that is not sent, and the reason to tell the user.
func (s *SSH) keepsHere(rel string) (bool, string) {
	switch {
	case !validSyncPath(rel):
		return true, "its name is unusual"
	case secretPath(rel):
		return true, "it looks like a secret"
	case s.opts.NoRead != nil && s.opts.NoRead(rel):
		return true, "a deny rule covers it"
	case settingPath(rel, s.acted):
		return true, "it is a tool's settings or installed code"
	}
	return false, ""
}

// localHash is the hash of a file here, by way of its size and time when those have
// not changed; "" when it is not a regular file reachable without a symlink.
func (s *SSH) localHash(rel string) string {
	abs := filepath.Join(s.host.Root(), filepath.FromSlash(rel))
	if symlinkOnPath(abs, []string{s.host.Root()}) != "" {
		return ""
	}
	fi, err := os.Lstat(abs)
	if err != nil || !fi.Mode().IsRegular() {
		return ""
	}
	if c, ok := s.stats[rel]; ok && c.size == fi.Size() && c.mod.Equal(fi.ModTime()) {
		return c.hash
	}
	// By its absolute path, which nothing reads as anything else.
	b, err := s.host.fs.ReadFile(abs)
	if err != nil {
		return ""
	}
	h := hashOf(b)
	s.stats[rel] = statHash{size: fi.Size(), mod: fi.ModTime(), hash: h}
	return h
}

// push sends what changed here since the two sides last agreed, and removes there what
// went from here. Secrets and what a deny rule covers stay here. The tar is written
// here, of regular files only, so nothing of this Mac is reachable through a link.
func (s *SSH) push(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()
	list, err := s.opts.ListFiles(ctx)
	if err != nil {
		return err
	}
	type sending struct {
		rel, hash string
		exec      bool
	}
	var send []sending
	present := map[string]bool{}
	var kept, links, large []string
	for _, rel := range list {
		if skip, why := s.keepsHere(rel); skip {
			kept = append(kept, rel+" ("+why+")")
			continue
		}
		abs := filepath.Join(s.host.Root(), filepath.FromSlash(rel))
		if symlinkOnPath(abs, []string{s.host.Root()}) != "" {
			links = append(links, rel)
			continue
		}
		fi, err := os.Lstat(abs)
		if err != nil {
			continue
		}
		if !fi.Mode().IsRegular() {
			continue
		}
		if fi.Size() > maxSyncFile {
			large = append(large, rel)
			continue
		}
		h := s.localHash(rel)
		if h == "" {
			continue
		}
		present[rel] = true
		if h != s.baseline[rel] {
			send = append(send, sending{rel: rel, hash: h, exec: fi.Mode().Perm()&0o111 != 0})
		}
	}
	s.tell("kept", kept, "Not copied to %s: %s.")
	s.tell("links", links, "Symbolic links are not copied to %s: %s.")
	s.tell("large", large, "Too large to copy to %s: %s.")

	var gone []string
	for rel := range s.baseline {
		if !present[rel] {
			gone = append(gone, rel)
		}
	}
	if len(send) > 0 {
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		written := 0
		flush := func() error {
			if written == 0 {
				return nil
			}
			if err := tw.Close(); err != nil {
				return err
			}
			if _, err := s.scriptBig(ctx, buf.Bytes(), `cd "$1" && exec tar -xof -`, s.work); err != nil {
				return err
			}
			buf.Reset()
			tw = tar.NewWriter(&buf)
			written = 0
			return nil
		}
		for _, f := range send {
			b, err := s.host.fs.ReadFile(filepath.Join(s.host.Root(), filepath.FromSlash(f.rel)))
			if err != nil {
				return fmt.Errorf("read %s: %w", f.rel, err)
			}
			mode := int64(0o644)
			if f.exec {
				mode = 0o755
			}
			if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "./" + f.rel, Mode: mode, Size: int64(len(b)), ModTime: time.Now()}); err != nil {
				return err
			}
			if _, err := tw.Write(b); err != nil {
				return err
			}
			written++
			if buf.Len() > 64<<20 {
				if err := flush(); err != nil {
					return err
				}
			}
		}
		if err := flush(); err != nil {
			return err
		}
		for _, f := range send {
			s.baseline[f.rel] = f.hash
			delete(s.said, "held:"+f.rel)
		}
	}
	if len(gone) > 0 {
		if err := s.removeThere(ctx, gone); err != nil {
			return err
		}
		for _, rel := range gone {
			delete(s.baseline, rel)
			delete(s.stats, rel)
		}
	}
	return nil
}

// removeThere deletes files of the working copy, named on standard input so that no
// name is read by a shell.
func (s *SSH) removeThere(ctx context.Context, rels []string) error {
	var names bytes.Buffer
	for _, rel := range rels {
		names.WriteString("./" + rel + "\x00")
	}
	_, err := s.scriptBig(ctx, names.Bytes(), `cd "$1" && exec xargs -0 rm -f --`, s.work)
	return err
}

// scriptBig is script for a large standard input.
func (s *SSH) scriptBig(ctx context.Context, stdin []byte, script string, args ...string) (ExecResult, error) {
	return s.script(ctx, syncTimeout, stdin, script, args...)
}

// tell says something about a set of paths once, and again only when the set grows.
func (s *SSH) tell(key string, paths []string, format string) {
	if len(paths) == 0 {
		return
	}
	if len(s.said) > maxSaid {
		clear(s.said)
	}
	said := strings.Join(paths, "\x00")
	if s.said[key+":"+said] {
		return
	}
	s.said[key+":"+said] = true
	if s.opts.Notify != nil {
		s.opts.Notify(fmt.Sprintf(format, s.opts.Host, listed(paths)))
	}
}

// listed names paths, not all of a long list.
func listed(paths []string) string {
	const shown = 12
	sorted := slices.Sorted(slices.Values(paths))
	if len(sorted) > shown {
		return fmt.Sprintf("%s and %d more", strings.Join(sorted[:shown], ", "), len(sorted)-shown)
	}
	return strings.Join(sorted, ", ")
}

// remoteLists are the names over there: every regular file, those changed since the
// marker, and what is neither a file nor a folder. They are data. Each is split at NUL.
type remoteLists struct {
	all, changed, other []string
}

// listScript prints the three lists with an empty name between them. It never
// descends into the working copy's own .git.
const listScript = `cd "$1" || exit 3
find . -name .git -prune -o -type f -print0
printf '\000'
[ -f "$2/m0" ] && find . -name .git -prune -o -type f "$3" "$2/m0" -print0
printf '\000'
find . -name .git -prune -o ! -type f ! -type d -print0
[ -f "$2/m1" ] && mv -f "$2/m1" "$2/m0"
exit 0`

func parseLists(out []byte) (remoteLists, error) {
	if len(out) > maxListBytes {
		return remoteLists{}, errors.New("the other machine listed more than can be right")
	}
	var lists remoteLists
	at := &lists.all
	section := 0
	names := strings.Split(string(out), "\x00")
	if len(names) > maxListNames {
		return remoteLists{}, fmt.Errorf("the other machine listed %d files, more than the %d that are looked at", len(names), maxListNames)
	}
	for _, name := range names {
		if name == "" {
			section++
			switch section {
			case 1:
				at = &lists.changed
			case 2:
				at = &lists.other
			default:
				at = nil
			}
			continue
		}
		if at != nil {
			*at = append(*at, strings.TrimPrefix(name, "./"))
		}
	}
	if section < 2 {
		return remoteLists{}, errors.New("the other machine's list of files was cut short")
	}
	return lists, nil
}

// readRemote runs a script and returns its standard output, up to limit.
func (s *SSH) readRemote(ctx context.Context, limit int64, stdin []byte, script string, args ...string) ([]byte, error) {
	var out bytes.Buffer
	err := s.streamRemote(ctx, stdin, func(r io.Reader) error {
		_, err := io.Copy(&out, io.LimitReader(r, limit+1))
		return err
	}, script, args...)
	if err != nil {
		return nil, err
	}
	if int64(out.Len()) > limit {
		return nil, errors.New("the other machine sent more than can be right")
	}
	return out.Bytes(), nil
}

// streamRemote runs a script and gives its standard output to read. It has its own
// process and not Spawn's, whose pipe closes when the process exits, with the end of a
// fast stream unread. A reader that stops early ends the process, and past the end of
// what it was to send a machine gets little more of a hearing, so one that keeps
// sending cannot hold this up.
func (s *SSH) streamRemote(ctx context.Context, stdin []byte, read func(io.Reader) error, script string, args ...string) error {
	argv := s.sshArgv(script, args...)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = "/"
	cmd.Env = baseEnv()
	cmd.Stdin = bytes.NewReader(stdin)
	pr, pw := io.Pipe()
	errOut := &capped{limit: 64 << 10}
	cmd.Stdout, cmd.Stderr = pw, errOut
	setProcessGroup(cmd)
	cmd.WaitDelay = waitDelay
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("environment: start %s: %w", cmd.Path, err)
	}
	readDone := make(chan error, 1)
	var flooded atomic.Bool
	go func() {
		err := read(pr)
		if err == nil {
			if n, _ := io.Copy(io.Discard, io.LimitReader(pr, 1<<20)); n >= 1<<20 {
				flooded.Store(true)
				killProcessGroup(cmd)
			}
		} else {
			killProcessGroup(cmd)
		}
		// A write after this fails, which ends the copy into pw.
		pr.CloseWithError(errors.New("no longer reading"))
		readDone <- err
	}()
	started := track(cmd.Process.Pid, cmd.Path)
	waited := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		stopGroup(cmd.Process.Pid)
		untrack(cmd.Process.Pid, started)
		waited <- err
	}()
	var waitErr error
	select {
	case waitErr = <-waited:
	case <-ctx.Done():
		killProcessGroup(cmd)
		waitErr = <-waited
	}
	pw.Close()
	readErr := <-readDone
	switch {
	case readErr != nil:
		return readErr
	case flooded.Load():
		return errors.New("the other machine kept sending after it was done")
	case ctx.Err() != nil:
		return errors.New("no answer in time")
	case waitErr != nil:
		said := strings.TrimSpace(errOut.String())
		if said == "" {
			said = waitErr.Error()
		}
		return s.connectError(ExecResult{Stderr: said})
	}
	return nil
}

// pull brings back what changed over there. It returns what the user and the agent
// should know. A file that does not come back keeps its baseline, so the next push does
// not take it for a change here.
func (s *SSH) pull(ctx context.Context) ([]string, error) {
	out, err := s.readRemote(ctx, maxListBytes, nil, listScript, s.work, s.state, s.newerFlag)
	if err != nil {
		return nil, err
	}
	lists, err := parseLists(out)
	if err != nil {
		return nil, err
	}
	everything := map[string]bool{}
	var odd []string
	wanted := map[string]bool{}
	consider := func(rel string) {
		if !validSyncPath(rel) {
			odd = append(odd, fmt.Sprintf("%q", rel))
			return
		}
		wanted[rel] = true
	}
	for _, rel := range lists.all {
		everything[rel] = true
		// New, or a name that moved here with its old time.
		if _, known := s.baseline[rel]; !known {
			consider(rel)
		}
	}
	for _, rel := range lists.changed {
		if everything[rel] {
			consider(rel)
		}
	}
	var notes []string
	if len(wanted) > maxPullFiles {
		return nil, fmt.Errorf("%d files changed, more than the %d that come back at once", len(wanted), maxPullFiles)
	}
	// A path through a link here is not written, and git refuses to judge one, for all the
	// others too.
	var throughLink []string
	for rel := range wanted {
		if symlinkOnPath(filepath.Join(s.host.Root(), filepath.FromSlash(rel)), []string{s.host.Root()}) != "" {
			throughLink = append(throughLink, rel)
			delete(wanted, rel)
		}
	}
	if fresh := s.fresh("link", throughLink); len(fresh) > 0 {
		notes = append(notes, fmt.Sprintf("Not brought back from %s, because a symbolic link here is on the way: %s.", s.opts.Host, listed(fresh)))
	}
	// Rules that cannot be read bring nothing back: what they would have kept out could be
	// a package that runs here. The list decides nothing about what is ignored by itself.
	ignored, err := s.ignoredOf(ctx, slices.Collect(mapKeys(wanted)))
	if err != nil {
		return nil, fmt.Errorf("read the project's ignore rules: %w", err)
	}
	for _, rel := range ignored {
		delete(wanted, rel)
	}
	var held []string
	for rel := range wanted {
		if s.holdBack(rel) != "" || s.bareRepoConfig(rel, lists.all) {
			held = append(held, rel)
			delete(wanted, rel)
		}
	}
	if len(wanted) > 0 {
		var names bytes.Buffer
		for _, rel := range slices.Sorted(mapKeys(wanted)) {
			names.WriteString("./" + rel + "\x00")
		}
		err := s.streamRemote(ctx, names.Bytes(), func(r io.Reader) error {
			more, err := s.applyTar(r, wanted)
			notes = append(notes, more...)
			return err
		}, `cd "$1" && COPYFILE_DISABLE=1 exec tar --null -T - -cf -`, s.work)
		if err != nil {
			return notes, err
		}
	}
	notes = append(notes, s.applyDeletes(everything)...)
	if fresh := s.fresh("held", held); len(fresh) > 0 {
		notes = append(notes, fmt.Sprintf("Held back on %s and not applied to the project here: %s. These are settings this Mac or an agent would act on, secrets, or paths a deny rule covers; copy them by hand if you want them.", s.opts.Host, listed(fresh)))
	}
	if fresh := s.fresh("odd", odd); len(fresh) > 0 {
		notes = append(notes, fmt.Sprintf("Not brought back from %s, because of their names: %s.", s.opts.Host, listed(fresh)))
	}
	var others []string
	for _, rel := range lists.other {
		others = append(others, fmt.Sprintf("%q", rel))
	}
	if fresh := s.fresh("other", others); len(fresh) > 0 {
		notes = append(notes, fmt.Sprintf("Only regular files come back, so these stay on %s: %s.", s.opts.Host, listed(fresh)))
	}
	return notes, nil
}

func mapKeys(m map[string]bool) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// fresh returns those of paths not yet told to the user.
func (s *SSH) fresh(kind string, paths []string) []string {
	if len(s.said) > maxSaid {
		clear(s.said)
	}
	var out []string
	for _, p := range paths {
		if !s.said[kind+":"+p] {
			s.said[kind+":"+p] = true
			out = append(out, p)
		}
	}
	return out
}

func (s *SSH) ignoredOf(ctx context.Context, rels []string) ([]string, error) {
	if s.opts.Ignored == nil || len(rels) == 0 {
		return nil, nil
	}
	return s.opts.Ignored(ctx, rels)
}

// bareRepoConfig reports a file named config beside a HEAD, here or over there, which
// together with objects and refs is a repository git would obey. A project can track a
// HEAD of its own, so holding HEAD back is not enough.
func (s *SSH) bareRepoConfig(rel string, remote []string) bool {
	if FoldPath(path.Base(rel)) != "config" {
		return false
	}
	dir := path.Dir(rel)
	if _, err := os.Lstat(filepath.Join(s.host.Root(), filepath.FromSlash(dir), "HEAD")); err == nil {
		return true
	}
	want := FoldPath(path.Join(dir, "HEAD"))
	return slices.ContainsFunc(remote, func(r string) bool { return FoldPath(r) == want })
}

// holdBack says why a path must not change here from over there, or "".
func (s *SSH) holdBack(rel string) string {
	switch {
	case heldPath(rel), settingPath(rel, s.acted):
		return "settings"
	case secretPath(rel):
		return "secret"
	case s.opts.NoWrite != nil && s.opts.NoWrite(rel):
		return "denied"
	case s.opts.NoRead != nil && s.opts.NoRead(rel):
		return "denied"
	}
	return ""
}

// applyTar reads a tar stream as data and applies the regular files in it that were
// asked for, each at most once. Everything else in the stream is skipped and counted:
// a sender with a different idea of what to send is itself worth telling.
func (s *SSH) applyTar(r io.Reader, wanted map[string]bool) ([]string, error) {
	var (
		notes            []string
		strange          int
		total            int64
		conflicts, large []string
		failed           []string
		seen             int
	)
	stop := func(err error) ([]string, error) {
		return s.syncNotes(notes, conflicts, large, failed, strange), err
	}
	// Entries it skips cost time too, so the whole stream has a limit.
	tr := tar.NewReader(io.LimitReader(r, maxPullTotal+maxPullFiles*4096))
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return stop(fmt.Errorf("read what came back: %w", err))
		}
		if seen++; seen > maxPullFiles+1000 {
			return stop(errors.New("the other machine sent more entries than were asked for"))
		}
		rel := strings.TrimPrefix(h.Name, "./")
		if h.Typeflag != tar.TypeReg || !wanted[rel] {
			// A directory entry is harmless; anything else was not asked for.
			if h.Typeflag != tar.TypeDir {
				strange++
			}
			continue
		}
		delete(wanted, rel) // once only
		if h.Size < 0 || h.Size > maxSyncFile {
			large = append(large, rel)
			continue
		}
		if total += h.Size; total > maxPullTotal {
			return stop(fmt.Errorf("more than %d MB changed at once", maxPullTotal>>20))
		}
		if s.pulled += h.Size; s.pulled > maxPullSession {
			return stop(fmt.Errorf("more than %d GB has come back in this session, which is more than can be right", maxPullSession>>30))
		}
		b, err := io.ReadAll(io.LimitReader(tr, maxSyncFile+1))
		if err != nil {
			return stop(fmt.Errorf("read %s: %w", rel, err))
		}
		if int64(len(b)) != h.Size {
			strange++
			continue
		}
		hash := hashOf(b)
		base, known := s.baseline[rel]
		if known && hash == base {
			continue
		}
		here := s.localHash(rel)
		abs := filepath.Join(s.host.Root(), filepath.FromSlash(rel))
		if here == hash {
			s.baseline[rel] = hash
			continue
		}
		exists := false
		if _, err := os.Lstat(abs); err == nil {
			exists = true
		}
		// Changed here too since the two sides agreed: the edit here stays, and the next
		// push sends it. A file that is new on both sides is the same case.
		if exists && (here != base || !known) {
			conflicts = append(conflicts, rel)
			continue
		}
		if link := symlinkOnPath(abs, []string{s.host.Root()}); link != "" {
			failed = append(failed, rel+" (through a symbolic link)")
			continue
		}
		if fi, err := os.Lstat(abs); err == nil && fi.Mode().Perm()&0o111 != 0 {
			failed = append(failed, rel+" (it is executable here, so it is not replaced)")
			continue
		}
		if err := s.setAside(rel); err != nil {
			failed = append(failed, rel+" ("+err.Error()+")")
			continue
		}
		if err := s.host.fs.WriteFile(abs, b); err != nil {
			failed = append(failed, rel+" ("+err.Error()+")")
			continue
		}
		s.baseline[rel] = hash
		delete(s.stats, rel)
	}
	return s.syncNotes(notes, conflicts, large, failed, strange), nil
}

func (s *SSH) syncNotes(notes, conflicts, large, failed []string, strange int) []string {
	if fresh := s.fresh("conflict", conflicts); len(fresh) > 0 {
		notes = append(notes, fmt.Sprintf("Changed both here and on %s; the version here stays and replaces the other: %s.", s.opts.Host, listed(fresh)))
	}
	if fresh := s.fresh("large", large); len(fresh) > 0 {
		notes = append(notes, fmt.Sprintf("Too large to bring back from %s: %s.", s.opts.Host, listed(fresh)))
	}
	if fresh := s.fresh("failed", failed); len(fresh) > 0 {
		notes = append(notes, fmt.Sprintf("Could not be brought back from %s: %s.", s.opts.Host, listed(fresh)))
	}
	if strange > 0 {
		notes = append(notes, fmt.Sprintf("%s sent %d entries that were not asked for, and they were ignored. Its tar may not be what it should be.", s.opts.Host, strange))
	}
	return notes
}

// applyDeletes removes here what went from over there, when the file here is still what
// both sides agreed on. Much going at once is not applied: the copy there gets the
// files back at the next push.
func (s *SSH) applyDeletes(remote map[string]bool) []string {
	var gone []string
	for rel := range s.baseline {
		if !remote[rel] {
			gone = append(gone, rel)
		}
	}
	if len(gone) == 0 {
		return nil
	}
	// Counted over the session too, or a little at a time would do the same.
	s.peakFiles = max(s.peakFiles, len(s.baseline))
	if total := s.deleted + len(gone); total > massDeleteCount && total > s.peakFiles/massDeleteShare {
		for _, rel := range gone {
			delete(s.baseline, rel)
		}
		return []string{fmt.Sprintf("%d files were deleted on %s, on top of %d earlier in this session, which was not applied here; they are copied back. Delete them here if that was meant.", len(gone), s.opts.Host, s.deleted)}
	}
	var held, kept, failed []string
	for _, rel := range gone {
		base := s.baseline[rel]
		delete(s.baseline, rel)
		switch {
		case s.holdBack(rel) != "":
			held = append(held, rel)
		case s.localHash(rel) != base:
			kept = append(kept, rel)
		default:
			if err := s.setAside(rel); err != nil {
				failed = append(failed, rel+" ("+err.Error()+")")
			} else if err := s.removeHere(rel); err != nil {
				failed = append(failed, rel+" ("+err.Error()+")")
			} else {
				s.deleted++
			}
			delete(s.stats, rel)
		}
	}
	var notes []string
	if fresh := s.fresh("heldgone", held); len(fresh) > 0 {
		notes = append(notes, fmt.Sprintf("Deleted on %s and kept here: %s.", s.opts.Host, listed(fresh)))
	}
	if fresh := s.fresh("keptgone", kept); len(fresh) > 0 {
		notes = append(notes, fmt.Sprintf("Deleted on %s but changed here, so kept: %s.", s.opts.Host, listed(fresh)))
	}
	if fresh := s.fresh("failedgone", failed); len(fresh) > 0 {
		notes = append(notes, fmt.Sprintf("Could not delete here what went on %s: %s.", s.opts.Host, listed(fresh)))
	}
	return notes
}

// setAside keeps a copy of a file about to be replaced or deleted from over there, in a
// folder of the session's, so what a command did to work not yet committed can be undone.
// With no room left the change does not happen.
func (s *SSH) setAside(rel string) error {
	if s.opts.SetAsideDir == "" {
		return nil
	}
	abs := filepath.Join(s.host.Root(), filepath.FromSlash(rel))
	fi, err := os.Lstat(abs)
	if err != nil || !fi.Mode().IsRegular() {
		return nil
	}
	if s.setAsideSize+fi.Size() > maxSetAside {
		return fmt.Errorf("the %d MB kept of what this session replaced is full", maxSetAside>>20)
	}
	b, err := s.host.fs.ReadFile(abs)
	if err != nil {
		return err
	}
	s.setAsideSeq++
	dst := filepath.Join(s.opts.SetAsideDir, fmt.Sprintf("%06d", s.setAsideSeq), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(dst, b, 0o600); err != nil {
		return err
	}
	s.setAsideSize += fi.Size()
	return nil
}

// removeHere deletes a regular file of the project, never through a link.
func (s *SSH) removeHere(rel string) error {
	abs := filepath.Join(s.host.Root(), filepath.FromSlash(rel))
	if link := symlinkOnPath(abs, []string{s.host.Root()}); link != "" {
		return fmt.Errorf("through a symbolic link")
	}
	if err := s.opts.Guard.CheckWrite(abs); err != nil {
		return err
	}
	fi, err := os.Lstat(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if !fi.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	return os.Remove(abs)
}

// clearStale removes from a copy left by an earlier run what the project here does not
// have, unless the project ignores it, so that it does not come back as new. A build
// folder stays. Nothing is brought back: the project here is what counts at a start.
func (s *SSH) clearStale(ctx context.Context) error {
	out, err := s.readRemote(ctx, maxListBytes, nil, listScript, s.work, s.state, s.newerFlag)
	if err != nil {
		return err
	}
	lists, err := parseLists(out)
	if err != nil {
		return err
	}
	var extra []string
	for _, rel := range lists.all {
		if _, ok := s.baseline[rel]; !ok {
			extra = append(extra, rel)
		}
	}
	if len(extra) == 0 {
		return nil
	}
	ignored, err := s.ignoredOf(ctx, extra)
	if err != nil {
		return err
	}
	keep := make(map[string]bool, len(ignored))
	for _, rel := range ignored {
		keep[rel] = true
	}
	extra = slices.DeleteFunc(extra, func(rel string) bool { return keep[rel] })
	if len(extra) == 0 {
		return nil
	}
	return s.removeThere(ctx, extra)
}

// initGit gives a copy with no repository one of its own, so that git status and diff
// work there against the start of the session. It is made there and never holds this
// project's history, remotes or credentials, and nothing comes back from it.
func (s *SSH) initGit(ctx context.Context) {
	s.script(ctx, 2*time.Minute, nil, `cd "$1" || exit 0
[ -e .git ] && exit 0
git init -q . || exit 0
git add -A >/dev/null 2>&1
git -c user.name=freeloop -c user.email=freeloop@invalid commit -q --no-verify -m "The session's start" >/dev/null 2>&1
exit 0`, s.work)
}
