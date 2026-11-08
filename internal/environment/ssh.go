package environment

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// An SSH environment runs a session's commands on another machine (ENV-008). The
// project stays on this Mac, where the app's git, diffs, checkpoints and file tools
// work as they do for a local folder; the session has a working copy over there.
//
// The other machine is not trusted, and neither is what runs on it. An approved
// command there can do anything to that machine, and may replace its tar and its find.
// So what comes back is data: this file and sshsync.go never run anything it says, and
// never hand it to a program here. See docs/SECURITY.md.

// KindSSH is a working copy on another machine.
const KindSSH Kind = "ssh"

// sshProgram is the system's own client, never one found on a PATH.
const sshProgram = "/usr/bin/ssh"

// stoppedPullGrace is how long what a stopped command changed may take to come back.
const stoppedPullGrace = 15 * time.Second

// DefaultRemoteDir holds a machine's working copies, under the remote home folder.
const DefaultRemoteDir = ".freeloop-sessions"

var (
	// A name or address for ssh, with a user. No space, quote or leading dash, so it
	// cannot be read as an option such as -oProxyCommand=. An IPv6 address needs a
	// Host entry in ~/.ssh/config.
	sshHostPattern = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._-]*@)?[A-Za-z0-9][A-Za-z0-9._-]*$`)
	// A folder over there: goes into a shell command, quoted, and ends up under rm -rf.
	remoteDirPattern = regexp.MustCompile(`^(~/|/)?[A-Za-z0-9._][A-Za-z0-9._/-]*$`)
	sshSessionID     = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	envNamePattern   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// ValidSSHHost reports whether host can be given to ssh as a destination.
func ValidSSHHost(host string) bool {
	return len(host) <= 255 && sshHostPattern.MatchString(host)
}

// validEnvName reports a variable a command can have. The __fl_ names are the script's
// own: __fl_cmd would replace the approved command.
func validEnvName(name string) bool {
	return envNamePattern.MatchString(name) && !strings.HasPrefix(name, "__fl_")
}

// ValidRemoteDir reports whether dir is a folder a session's copy can go under.
func ValidRemoteDir(dir string) bool {
	if len(dir) > 1024 || !remoteDirPattern.MatchString(dir) {
		return false
	}
	return !slices.Contains(strings.Split(dir, "/"), "..")
}

// SSHOptions configures an SSH environment.
type SSHOptions struct {
	ID string
	// Host is what ssh connects to: a name from ~/.ssh/config, or [user@]host.
	Host string
	// RemoteDir holds the session's folder; DefaultRemoteDir when empty.
	RemoteDir string
	// SessionID names the session's folder there, which is all that is ever deleted.
	SessionID string
	// Root is the project on this Mac.
	Root string
	// Env reaches every command, by way of its standard input: never on a command line
	// here or there, and never in the environment of ssh.
	Env map[string]string
	// Setup runs once for a working copy, Health before every session start.
	Setup, Health string
	Guard         Guard
	// ControlDir holds the shared connection's socket; empty means no sharing. It must
	// be private to the user.
	ControlDir string
	// ConfigFile replaces ~/.ssh/config, for tests.
	ConfigFile string
	// Allowed is asked before anything goes to the machine: local-only mode can come on
	// while a session runs.
	Allowed func() error

	// ListFiles lists the project's files to send, relative to Root: what git tracks or
	// would track, so what the project ignores stays here.
	ListFiles func(ctx context.Context) ([]string, error)
	// Ignored returns those of paths the project ignores, which are not brought back.
	// An error brings nothing back, since what it would have kept out could run here.
	Ignored func(ctx context.Context, paths []string) ([]string, error)
	// NoRead reports a path that must not leave this Mac: a deny rule covers reading it.
	NoRead func(rel string) bool
	// NoWrite reports a path that must not change from over there: a deny rule covers it.
	NoWrite func(rel string) bool
	// ActedOn names files and folders something on this Mac acts on, which cross neither
	// way, on top of the ones here (permission.ActedOnLater).
	ActedOn []string
	// SetAsideDir keeps what comes back's predecessors: a private folder of the session's.
	SetAsideDir string
	// Notify tells the user what the sync held back or could not do.
	Notify func(message string)
}

// SSH is a working copy on another machine. Commands run one at a time, each with the
// copy brought up to date first and what it changed brought back after.
type SSH struct {
	opts SSHOptions
	// host is this Mac's side: the project's files, and the ssh processes. It has no
	// variables of its own, so nothing of the session's reaches ssh's environment.
	host *Local

	// mu is held for a whole command, so tools reading paths use published.
	mu        sync.Mutex
	published atomic.Pointer[string] // work, once it is known
	base      string                 // the session's folder over there, absolute
	work      string
	state     string
	acted     map[string]bool
	synced

	// master is the shared connection's process once seen, with when it started; Close
	// ends it (noteMaster).
	masterMu sync.Mutex
	master   int
	masterAt time.Time
}

// NewSSH returns an SSH environment; Start connects and prepares the working copy.
func NewSSH(opts SSHOptions) (*SSH, error) {
	if !ValidSSHHost(opts.Host) {
		return nil, fmt.Errorf("environment: %q is not a host ssh can connect to", opts.Host)
	}
	if opts.RemoteDir == "" {
		opts.RemoteDir = DefaultRemoteDir
	}
	if !ValidRemoteDir(opts.RemoteDir) {
		return nil, fmt.Errorf("environment: %q is not a folder a session's copy can go under", opts.RemoteDir)
	}
	if !sshSessionID.MatchString(opts.SessionID) {
		return nil, errors.New("environment: an SSH environment needs a session")
	}
	if opts.ListFiles == nil {
		return nil, errors.New("environment: an SSH environment needs a list of the project's files")
	}
	if _, err := os.Stat(sshProgram); err != nil {
		return nil, fmt.Errorf("the ssh command is not at %s", sshProgram)
	}
	host, err := NewLocal(LocalOptions{ID: opts.ID, Root: opts.Root, Guard: opts.Guard})
	if err != nil {
		return nil, err
	}
	for name := range opts.Env {
		if !validEnvName(name) {
			return nil, fmt.Errorf("environment: %q cannot be a variable here", name)
		}
	}
	return &SSH{opts: opts, host: host, synced: newSynced(), acted: nameSet(opts.ActedOn...)}, nil
}

func (s *SSH) ID() string     { return s.opts.ID }
func (s *SSH) Kind() Kind     { return KindSSH }
func (s *SSH) Root() string   { return s.host.Root() }
func (s *SSH) FS() FileSystem { return s.host.FS() }
func (s *SSH) Host() string   { return s.opts.Host }

// Resolve also takes a path of the working copy, as a command's output names them.
func (s *SSH) Resolve(p string) string {
	return s.host.Resolve(s.mapWork(p))
}

func (s *SSH) Contains(p string) bool { return s.host.Contains(s.Resolve(p)) }

// Describe says where commands run, for the agent's system prompt.
func (s *SSH) Describe() string {
	work := ""
	if w := s.published.Load(); w != nil {
		work = *w
	}
	return fmt.Sprintf("Shell commands run on %s, in %s, a working copy of the project root that is kept in sync: the same files at the same relative paths. "+
		"Use relative paths in commands; the project root is not a path on that machine. File tools work on the project root. "+
		"What the project ignores stays on the machine it is on, and some files are not synced: secrets, git's own files and agent settings.", s.opts.Host, work)
}

// sshOptions are given on the command line, which wins over ~/.ssh/config, so a config
// or a Host entry cannot turn them off.
//
// Host keys are never accepted here: an unknown or changed key refuses to connect, and
// the user checks it in a terminal. Nothing of this Mac reaches the other machine: its
// agent, X, or ports either way. It cannot run a command here either.
var sshOptions = []string{
	"-T", "-a", "-x", "-e", "none",
	"-o", "BatchMode=yes",
	"-o", "StrictHostKeyChecking=yes",
	"-o", "UpdateHostKeys=no",
	"-o", "ForwardAgent=no",
	"-o", "ForwardX11=no",
	"-o", "ClearAllForwardings=yes",
	"-o", "Tunnel=no",
	"-o", "PermitLocalCommand=no",
	"-o", "RemoteCommand=none",
	"-o", "RequestTTY=no",
	"-o", "SendEnv=-*",
	"-o", "ConnectTimeout=15",
	"-o", "ServerAliveInterval=15",
	"-o", "ServerAliveCountMax=4",
	"-o", "LogLevel=ERROR",
}

// sshArgv is an ssh command line for a script run by sh on the other machine. ssh
// joins what follows the host with spaces for a shell over there, so every word is
// quoted here.
func (s *SSH) sshArgv(script string, args ...string) []string {
	argv := append([]string{sshProgram}, sshOptions...)
	if s.opts.ConfigFile != "" {
		argv = append(argv, "-F", s.opts.ConfigFile)
	}
	if socket := s.controlSocket(); socket != "" {
		argv = append(argv, "-o", "ControlMaster=auto", "-o", "ControlPath="+socket, "-o", "ControlPersist=60")
	} else {
		// Not the user's own sharing either: a connection of theirs could have what this
		// command line turns off.
		argv = append(argv, "-o", "ControlMaster=no", "-o", "ControlPath=none")
	}
	words := []string{"sh", "-c", quoteSh(script), "freeloop"}
	for _, a := range args {
		words = append(words, quoteSh(a))
	}
	return append(argv, "--", s.opts.Host, strings.Join(words, " "))
}

// controlSocket is the shared connection's socket as ssh is given it, with %C for ssh to
// fill in; empty when the session shares no connection. One per session, since ending
// it ends every command using it. Sockets have a short limit on their path, and %C is
// 40 characters.
func (s *SSH) controlSocket() string {
	if s.opts.ControlDir == "" {
		return ""
	}
	socket := filepath.Join(s.opts.ControlDir, s.socketPrefix()+"-%C")
	if len(socket)+38 >= 100 {
		return ""
	}
	return socket
}

func (s *SSH) socketPrefix() string { return hashOf([]byte(s.opts.SessionID))[:12] }

// controlArgv asks the shared connection's master to do op, check or exit: the same
// options as a command, with -O op in place of the command.
func (s *SSH) controlArgv(op string) []string {
	argv := s.sshArgv("")
	return append(argv[:len(argv)-3:len(argv)-3], "-O", op, "--", s.opts.Host)
}

// masterPID is in what ssh -O check says.
var masterPID = regexp.MustCompile(`\(pid=([0-9]+)\)`)

// noteMaster records the shared connection's master in the process ledger. ssh puts
// it in a session and process group of its own, out of the group of the command that
// started it, so neither that group's end nor a crash's leftovers would include it.
// A master that ControlPersist ended is replaced by the next command, so this runs
// after each one; while the known master runs it costs nothing.
func (s *SSH) noteMaster(ctx context.Context) {
	if s.controlSocket() == "" {
		return
	}
	s.masterMu.Lock()
	defer s.masterMu.Unlock()
	if s.master != 0 {
		if s.masterRunning() {
			return
		}
		untrack(s.master, s.masterAt)
		s.master, s.masterAt = 0, time.Time{}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	res, err := s.host.Exec(ctx, ExecSpec{Argv: s.controlArgv("check"), Cwd: "/", Timeout: 10 * time.Second, MaxOutput: 4 << 10, SeparateStderr: true})
	if err != nil || res.ExitCode != 0 {
		return
	}
	m := masterPID.FindStringSubmatch(res.Stderr + res.Output)
	if m == nil {
		return
	}
	pid, err := strconv.Atoi(m[1])
	if err != nil || pid <= 1 {
		return
	}
	s.master, s.masterAt = pid, track(pid, sshProgram)
}

// masterRunning reports whether the recorded master still runs, and is the very
// process recorded where the platform can tell; called with masterMu held.
func (s *SSH) masterRunning() bool {
	if ProcessStarted != nil && !s.masterAt.IsZero() {
		at, ok := ProcessStarted(s.master)
		return ok && at.Equal(s.masterAt)
	}
	return groupAlive(s.master)
}

// quoteSh quotes s for a POSIX shell.
func quoteSh(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// assignLine is a line of shell that sets a variable, with no newline or quote of its
// value in it: printf's octal escapes carry any bytes, and the x keeps $(...) from
// taking trailing newlines off.
func assignLine(name, value string, export bool) string {
	var b strings.Builder
	b.WriteString("__fl_v=$(printf '")
	for i := 0; i < len(value); i++ {
		fmt.Fprintf(&b, `\%03o`, value[i])
	}
	b.WriteString("x'); ")
	b.WriteString(name + `="${__fl_v%x}"`)
	if export {
		b.WriteString("; export " + name)
	}
	return b.String()
}

// prepareScript makes the session's folder only its owner can enter, and prints where
// it is and what the machine has.
const prepareScript = `umask 077
cd || exit 3
mkdir -p "$1" || exit 3
for link in "$1/freeloop-$2" "$1/freeloop-$2/work" "$1/freeloop-$2/state"; do
  if [ -L "$link" ]; then echo "the session's folder is a symbolic link" >&2; exit 4; fi
done
mkdir -p "$1/freeloop-$2/work" "$1/freeloop-$2/state" || exit 3
chmod 700 "$1/freeloop-$2" || exit 3
cd "$1/freeloop-$2" || exit 3
for tool in tar find xargs rm mv; do
  command -v "$tool" >/dev/null 2>&1 || { echo "this machine has no $tool" >&2; exit 5; }
done
: > state/m0
if find state -cnewer state/m0 -prune >/dev/null 2>&1; then c=cnewer; else c=newer; fi
if command -v git >/dev/null 2>&1; then g=git; else g=none; fi
if [ -e state/setup-done ]; then d=done; else d=new; fi
printf '%s\n%s\n%s\n%s\n' "$c" "$g" "$d" "$(pwd -P)"`

// Start connects, prepares the working copy and brings it up to date. It returns what
// it did, for the transcript.
func (s *SSH) Start(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.allowed(); err != nil {
		return "", err
	}
	res, err := s.host.Exec(ctx, ExecSpec{Argv: s.sshArgv(prepareScript, s.opts.remoteDirArg(), s.opts.SessionID), Cwd: "/", Timeout: 2 * time.Minute, MaxOutput: 64 << 10, SeparateStderr: true})
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", s.connectError(res)
	}
	s.noteMaster(ctx)
	lines := strings.Split(strings.TrimSpace(res.Output), "\n")
	if len(lines) < 4 {
		return "", errors.New("the other machine did not answer as expected")
	}
	// What the other machine says is not trusted to be a path. Startup files may also
	// print first, so this reads from the end.
	lines = lines[len(lines)-4:]
	base := lines[3]
	if !strings.HasPrefix(base, "/") || !ValidRemoteDir(base) || path.Base(base) != "freeloop-"+s.opts.SessionID {
		return "", fmt.Errorf("the other machine named %q as the session's folder, which is not used", base)
	}
	s.base, s.work, s.state = base, base+"/work", base+"/state"
	s.published.Store(&s.work)
	s.newerFlag = "-newer"
	if lines[0] == "cnewer" {
		s.newerFlag = "-cnewer"
	}
	var did []string
	did = append(did, "connected to "+s.opts.Host)
	if err := s.push(ctx); err != nil {
		return strings.Join(did, ", "), fmt.Errorf("copy the project to %s: %w", s.opts.Host, err)
	}
	if err := s.clearStale(ctx); err != nil {
		return strings.Join(did, ", "), fmt.Errorf("tidy the working copy on %s: %w", s.opts.Host, err)
	}
	if _, err := s.script(ctx, 30*time.Second, nil, `: > "$1/m0"`, s.state); err != nil {
		return strings.Join(did, ", "), err
	}
	did = append(did, "brought its working copy up to date")
	if lines[1] == "git" {
		s.initGit(ctx)
	}
	if s.opts.Setup != "" && lines[2] != "done" {
		out, err := s.run(ctx, ExecSpec{Command: s.opts.Setup, Timeout: MaxTimeout, MaxOutput: 64 << 10})
		if err != nil || out.ExitCode != 0 {
			return strings.Join(did, ", "), fmt.Errorf("the setup command failed: %s", lastLines(out.Output, 6))
		}
		if _, err := s.script(ctx, 30*time.Second, nil, `: > "$1/setup-done"`, s.state); err != nil {
			return strings.Join(did, ", "), err
		}
		did = append(did, "ran the setup command")
	}
	if s.opts.Health != "" {
		out, err := s.run(ctx, ExecSpec{Command: s.opts.Health, Timeout: 2 * time.Minute})
		if err != nil || out.ExitCode != 0 {
			return strings.Join(did, ", "), fmt.Errorf("the environment is unhealthy: %s", lastLines(out.Output, 6))
		}
	}
	return strings.Join(did, ", "), nil
}

// remoteDirArg is the folder for a script that starts in the home folder, where ~/
// means no more than a relative path.
func (o SSHOptions) remoteDirArg() string { return strings.TrimPrefix(o.RemoteDir, "~/") }

// connectError says what to do about a connection that failed.
func (s *SSH) connectError(res ExecResult) error {
	said := strings.TrimSpace(res.Stderr + "\n" + res.Output)
	switch {
	case strings.Contains(said, "Host key verification failed"), strings.Contains(said, "REMOTE HOST IDENTIFICATION HAS CHANGED"), strings.Contains(said, "No ED25519 host key is known"), strings.Contains(said, "host key is known"):
		return fmt.Errorf("%s has a host key this Mac does not know or that has changed, and Freeloop does not accept one by itself. Run ssh %s in a terminal, check the fingerprint it shows, then try again", s.opts.Host, s.opts.Host)
	case strings.Contains(said, "Permission denied"):
		return fmt.Errorf("%s refused the login. Freeloop signs in with no prompt, using your keys and ssh agent as ssh does: check that ssh %s works in a terminal without a password", s.opts.Host, s.opts.Host)
	}
	return fmt.Errorf("connect to %s: %s", s.opts.Host, lastLines(said, 4))
}

// script runs a script over there with no variables of the session.
func (s *SSH) script(ctx context.Context, timeout time.Duration, stdin []byte, script string, args ...string) (ExecResult, error) {
	res, err := s.host.Exec(ctx, ExecSpec{Argv: s.sshArgv(script, args...), Cwd: "/", Timeout: timeout, MaxOutput: 64 << 10, Stdin: stdin, SeparateStderr: true})
	if err != nil {
		return res, err
	}
	s.noteMaster(ctx)
	if res.TimedOut {
		return res, errors.New("no answer in time")
	}
	if res.ExitCode != 0 {
		return res, s.connectError(res)
	}
	return res, nil
}

// execScript reads what it must not have on a command line from standard input, one
// line at a time, so what follows stays for the command: read takes no more than a
// line. It runs under timeout where there is one, so a deadline holds when the
// connection does not.
const execScript = `IFS= read -r __fl_n || exit 125
__fl_i=0
while [ "$__fl_i" -lt "$__fl_n" ]; do
  IFS= read -r __fl_l || exit 125
  eval "$__fl_l"
  __fl_i=$((__fl_i+1))
done
: > "$1/m1"
cd "$__fl_cwd" || exit 125
echo "$$" > "$1/run-$2"
if command -v timeout >/dev/null 2>&1; then timeout -s KILL "$3" sh -lc "$__fl_cmd"; else sh -lc "$__fl_cmd"; fi
__fl_rc=$?
rm -f "$1/run-$2"
exit "$__fl_rc"`

// sshKillScript ends a command and what it started.
const sshKillScript = `kill_tree() {
  for c in $(ps -A -o pid= -o ppid= | awk -v p="$1" '$2 == p { print $1 }'); do kill_tree "$c"; done
  kill -KILL "$1" 2>/dev/null
}
[ -f "$1" ] || exit 0
read -r p < "$1" && [ -n "$p" ] && kill_tree "$p"
rm -f "$1"`

// Exec brings the working copy up to date, runs the command there and brings back what
// it changed. A command that only reads moves no files. The broker has approved the
// command already: nothing here is a reason to skip it.
func (s *SSH) Exec(ctx context.Context, spec ExecSpec) (ExecResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.work == "" {
		return ExecResult{}, errors.New("the working copy is not ready")
	}
	if err := s.allowed(); err != nil {
		return ExecResult{}, err
	}
	if !spec.ReadOnly {
		if err := s.push(ctx); err != nil {
			return ExecResult{}, fmt.Errorf("bring the working copy on %s up to date: %w", s.opts.Host, err)
		}
	}
	res, err := s.run(ctx, spec)
	if err != nil || spec.ReadOnly {
		return res, err
	}
	// What a stopped command changed still comes back, or the two copies would differ
	// with no one knowing. A stop gets a little time for that, not the whole limit: a
	// machine that stalls must not hold the session.
	pullCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), syncTimeout)
	defer cancel()
	stopped := context.AfterFunc(ctx, func() {
		select {
		case <-time.After(stoppedPullGrace):
			cancel()
		case <-pullCtx.Done():
		}
	})
	defer stopped()
	notes, pullErr := s.pull(pullCtx)
	if pullErr != nil {
		notes = append(notes, fmt.Sprintf("What the command changed on %s could not be brought back: %v", s.opts.Host, pullErr))
	}
	if len(notes) > 0 {
		said := strings.Join(notes, "\n")
		if s.opts.Notify != nil {
			s.opts.Notify(said)
		}
		res.Output = strings.TrimRight(res.Output, "\n") + "\n[Freeloop] " + said
	}
	return res, nil
}

func (s *SSH) allowed() error {
	if s.opts.Allowed == nil {
		return nil
	}
	return s.opts.Allowed()
}

// run runs one command over there, with no sync.
func (s *SSH) run(ctx context.Context, spec ExecSpec) (ExecResult, error) {
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	timeout = min(timeout, MaxTimeout)
	cwd, err := s.remoteCwd(spec.Cwd)
	if err != nil {
		return ExecResult{}, err
	}
	command := spec.Command
	if len(spec.Argv) > 0 {
		words := make([]string, len(spec.Argv))
		for i, a := range spec.Argv {
			words[i] = quoteSh(a)
		}
		command = "exec " + strings.Join(words, " ")
	}
	vars := maps.Clone(s.opts.Env)
	if vars == nil {
		vars = map[string]string{}
	}
	maps.Copy(vars, spec.Env)
	lines := []string{assignLine("__fl_cwd", cwd, false), assignLine("__fl_cmd", command, false)}
	for _, name := range slices.Sorted(maps.Keys(vars)) {
		if !validEnvName(name) {
			return ExecResult{}, fmt.Errorf("environment: %q cannot be a variable here", name)
		}
		lines = append(lines, assignLine(name, vars[name], true))
	}
	stdin := []byte(fmt.Sprintf("%d\n%s\n", len(lines), strings.Join(lines, "\n")))
	stdin = append(stdin, spec.Stdin...)
	id := newExecID()
	res, err := s.host.Exec(ctx, ExecSpec{Argv: s.sshArgv(execScript, s.state, id, fmt.Sprint(int(timeout.Seconds()))), Cwd: "/",
		Timeout: timeout + 5*time.Second, MaxOutput: spec.MaxOutput, Stdin: stdin, SeparateStderr: spec.SeparateStderr})
	// Ending ssh here leaves the command running there.
	if ctx.Err() != nil || res.TimedOut {
		killCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		s.host.Exec(killCtx, ExecSpec{Argv: s.sshArgv(sshKillScript, s.state+"/run-"+id), Cwd: "/", Timeout: 20 * time.Second})
	}
	s.noteMaster(ctx)
	if err == nil && res.ExitCode == 137 && res.Duration >= timeout {
		res.TimedOut = true
	}
	if err == nil && res.ExitCode == 255 && strings.Contains(res.Output+res.Stderr, "Host key verification failed") {
		return res, s.connectError(res)
	}
	return res, err
}

// remoteCwd turns a command's folder into the same one in the working copy. A folder
// outside the project has no counterpart.
func (s *SSH) remoteCwd(cwd string) (string, error) {
	if cwd == "" {
		return s.work, nil
	}
	abs := filepath.Clean(s.host.Resolve(s.mapWork(cwd)))
	rel, err := filepath.Rel(s.host.Root(), abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside the project, so it has no place in the working copy", cwd)
	}
	return path.Join(s.work, filepath.ToSlash(rel)), nil
}

// mapWork turns a path of the working copy into the same one here. Any other path
// stays as it is, and is judged as a path of this Mac.
func (s *SSH) mapWork(p string) string {
	w := s.published.Load()
	if w == nil {
		return p
	}
	if c := path.Clean(p); c == *w || strings.HasPrefix(c, *w+"/") {
		return filepath.Join(s.host.Root(), filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(c, *w), "/")))
	}
	return p
}

// Close ends the shared connection: it asks the master to exit, stops it by its process
// group if it is still there a moment later, and removes a socket it left. ControlPersist
// keeps the connection for the next command while the session runs; it does not outlive
// the session, or the app.
func (s *SSH) Close(ctx context.Context) {
	if s.controlSocket() == "" {
		return
	}
	s.host.Exec(ctx, ExecSpec{Argv: s.controlArgv("exit"), Cwd: "/", Timeout: 10 * time.Second, MaxOutput: 4 << 10})
	s.masterMu.Lock()
	if s.master != 0 {
		// It exits as soon as it has answered; one that did not hear goes with its group.
		for deadline := time.Now().Add(2 * time.Second); s.masterRunning() && time.Now().Before(deadline); {
			time.Sleep(20 * time.Millisecond)
		}
		if s.masterRunning() {
			stopGroup(s.master)
		}
		untrack(s.master, s.masterAt)
		s.master, s.masterAt = 0, time.Time{}
	}
	s.masterMu.Unlock()
	// A master that was killed leaves its socket behind. Only this session's: the name
	// starts with its own prefix, and only a socket goes.
	sockets, _ := filepath.Glob(filepath.Join(s.opts.ControlDir, s.socketPrefix()+"-*"))
	for _, p := range sockets {
		if fi, err := os.Lstat(p); err == nil && fi.Mode().Type() == os.ModeSocket {
			os.Remove(p)
		}
	}
}

// RemoveSSHCopy deletes a session's folder on the machine it was on. Only that folder
// can go: its name is built here from a checked session ID.
func RemoveSSHCopy(ctx context.Context, opts SSHOptions) error {
	s, err := NewSSH(opts)
	if err != nil {
		return err
	}
	_, err = s.script(ctx, 2*time.Minute, nil, `cd || exit 3; rm -rf -- "$1/freeloop-$2"`, s.opts.remoteDirArg(), s.opts.SessionID)
	s.Close(ctx)
	return err
}

// ResolvedSSHHost asks ssh, with no connection, what address host means after
// ~/.ssh/config. Local-only mode goes by this.
func ResolvedSSHHost(ctx context.Context, host, configFile string) (string, error) {
	if !ValidSSHHost(host) {
		return "", fmt.Errorf("%q is not a host ssh can connect to", host)
	}
	args := []string{sshProgram, "-G"}
	if configFile != "" {
		args = append(args, "-F", configFile)
	}
	l, err := NewLocal(LocalOptions{Root: "/", Guard: allowAll{}})
	if err != nil {
		return "", err
	}
	res, err := l.Exec(ctx, ExecSpec{Argv: append(args, "--", host), Timeout: 10 * time.Second, MaxOutput: 256 << 10})
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("ssh -G %s: %s", host, lastLines(res.Output, 2))
	}
	var hostname string
	proxied := false
	for _, line := range strings.Split(res.Output, "\n") {
		k, v, _ := strings.Cut(line, " ")
		switch strings.ToLower(k) {
		case "hostname":
			hostname = strings.TrimSpace(v)
		case "proxycommand", "proxyjump":
			if v = strings.TrimSpace(v); v != "" && v != "none" {
				proxied = true
			}
		}
	}
	if proxied {
		return "", fmt.Errorf("%s goes through a proxy, so where it leads cannot be told from here", host)
	}
	if hostname == "" {
		return "", fmt.Errorf("ssh -G %s named no address", host)
	}
	return hostname, nil
}

// allowAll is for running ssh itself, which touches no project file.
type allowAll struct{}

func (allowAll) CheckRead(string) error  { return nil }
func (allowAll) CheckWrite(string) error { return nil }
