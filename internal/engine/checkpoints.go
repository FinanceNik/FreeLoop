package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
)

// Checkpoints (SES-014): before each prompt the session's folder is
// snapshotted into a git store of the session's own, in its data folder
// (not the project's repository, which is never touched, and which need not
// exist). The store honours the project's .gitignore, so dependencies and
// build output stay out. "Rewind to here" restores the files of a
// checkpoint, after snapshotting the current state, and cuts the built-in
// harness's context back to that point.

// checkpointTimeout bounds one snapshot; a folder too big to snapshot in
// time turns checkpoints off for the session.
const checkpointTimeout = 90 * time.Second

// History is implemented by harness sessions whose context can be cut back
// to a length (the built-in harness).
type History interface {
	HistoryLen() int
	TruncateHistory(n int)
}

func (e *Engine) checkpointStore(sid string) string {
	return filepath.Join(e.dataDir, "sessions", sid, "checkpoints.git")
}

// shadowBase is the start of a git command on the session's checkpoint store.
func (e *Engine) shadowBase(sid string, host *environment.Local) []string {
	return []string{"--git-dir=" + e.checkpointStore(sid), "--work-tree=" + host.Root(), "-c", "user.name=Freeloop", "-c", "user.email=freeloop@localhost", "-c", "commit.gpgsign=false"}
}

// shadowGit runs git on the session's checkpoint store with its folder as
// the work tree. Hooks, fsmonitor and the user's identity stay out.
func (e *Engine) shadowGit(ctx context.Context, host *environment.Local, sid string, timeout time.Duration, args ...string) (string, error) {
	res, err := gitRun(ctx, host, timeout, append(e.shadowBase(sid, host), args...)...)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(res.Output))
	}
	return strings.TrimSpace(res.Output), nil
}

// snapshotExclusions keeps .claude and .freeloop out of a snapshot. git add refuses a
// pathspec naming a folder that is ignored as a whole, even as an exclusion, though it
// adds everything else: that turned checkpoints off for a project ignoring .claude. So
// one ignored as a whole is left to the ignore rule, which nothing below it can undo.
// One ignored in part keeps its exclusion, since .claude/* with !.claude/commands/
// would bring files in. When git cannot say, the exclusions stay: at worst that is a
// snapshot failing, and never one holding these folders.
func (e *Engine) snapshotExclusions(ctx context.Context, host *environment.Local, sid string) []string {
	// Asked without a final slash: with one, .claude/* counts as ignoring the folder
	// itself, and its ! lines would then bring files in. Git sees for itself that it is
	// a folder, which is what .claude/ as a rule needs.
	folders := []string{".claude", ProjectDir}
	ignored := ""
	res, err := gitRun(ctx, host, 30*time.Second, append(e.shadowBase(sid, host), append([]string{"check-ignore", "--"}, folders...)...)...)
	// It exits 0 with some ignored and 1 with none.
	if err == nil && res.ExitCode == 0 {
		ignored = res.Output
	}
	out := []string{}
	for _, f := range folders {
		if !slices.Contains(strings.Fields(ignored), f) {
			out = append(out, ":(exclude)"+f)
		}
	}
	return out
}

// snapshot commits the folder's current state and returns the commit.
func (e *Engine) snapshot(ctx context.Context, host *environment.Local, sid, message string) (string, error) {
	if !environment.IsDir(e.checkpointStore(sid)) {
		if _, err := gitRun(ctx, host, 30*time.Second, "init", "-q", "--bare", e.checkpointStore(sid)); err != nil {
			return "", err
		}
	}
	// A nested repository stays out too. For one the index knows, git add
	// starts git in it to see whether it changed, under that repository's own
	// config, which can run programs and which a container can write. A
	// snapshot never held its files anyway, only the commit it stood at.
	if err := e.dropNested(ctx, host, sid); err != nil {
		return "", err
	}
	// The project's .claude/ is never written (a hard rule) and .freeloop/
	// holds the user's own settings and deny rules: snapshots leave both out,
	// so a rewind never changes them.
	if _, err := e.shadowGit(ctx, host, sid, checkpointTimeout, "rm", "-r", "-q", "--cached", "--ignore-unmatch", "--", ".claude", ProjectDir); err != nil {
		return "", err
	}
	if _, err := e.shadowGit(ctx, host, sid, checkpointTimeout, append([]string{"add", "-A", "--", "."}, e.snapshotExclusions(ctx, host, sid)...)...); err != nil {
		return "", err
	}
	if err := e.dropNested(ctx, host, sid); err != nil {
		return "", err
	}
	if _, err := e.shadowGit(ctx, host, sid, 30*time.Second, "commit", "-q", "--allow-empty", "--no-verify", "-m", message); err != nil {
		return "", err
	}
	return e.shadowGit(ctx, host, sid, 10*time.Second, "rev-parse", "HEAD")
}

// dropNested takes the nested repositories out of the store's index.
func (e *Engine) dropNested(ctx context.Context, host *environment.Local, sid string) error {
	ctx, cancel := context.WithTimeout(ctx, checkpointTimeout)
	defer cancel()
	store := "--git-dir=" + e.checkpointStore(sid)
	res, err := host.Exec(ctx, environment.ExecSpec{
		Argv: append(slices.Clone(safeGit), store, "--work-tree="+host.Root(), "ls-files", "--stage", "-z"),
		Env:  gitEnv(nil), MaxOutput: 64 << 20, Timeout: checkpointTimeout,
	})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("git ls-files: %s", strings.TrimSpace(res.Output))
	}
	var nested []string
	for _, entry := range strings.Split(res.Output, "\x00") {
		if mode, rest, ok := strings.Cut(entry, " "); ok && mode == "160000" {
			if _, p, ok := strings.Cut(rest, "\t"); ok {
				nested = append(nested, p)
			}
		}
	}
	for chunk := range slices.Chunk(nested, 200) {
		if _, err := e.shadowGit(ctx, host, sid, 30*time.Second, append([]string{"update-index", "--force-remove", "--"}, chunk...)...); err != nil {
			return err
		}
	}
	return nil
}

// checkpoint records the state before a prompt. A failure turns checkpoints
// off for the runtime and says so once.
func (e *Engine) checkpoint(r *runtime) {
	if r.noCheckpoints {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), checkpointTimeout+time.Minute)
	defer cancel()
	commit, err := e.snapshot(ctx, r.host, r.sid, "before a turn")
	if err != nil {
		r.noCheckpoints = true
		log.Printf("engine: checkpoint %s: %v", r.sid, err)
		e.emit(r.sid, event.Notice, event.NoticeData{Message: "Checkpoints are off for this session: " + err.Error()})
		return
	}
	d := event.CheckpointData{Commit: commit}
	if h, ok := r.hs.(History); ok {
		d.HistoryLen = h.HistoryLen()
	}
	e.emit(r.sid, event.Checkpoint, d)
}

// Rewind restores the files of the checkpoint recorded at seq, after a
// snapshot of the current state, and cuts the built-in harness's context
// back to that point. Turns after it stay in the transcript, marked as
// rewound by the rewind event.
func (e *Engine) Rewind(ctx context.Context, sid string, seq int64) error {
	events, err := e.store.Events(ctx, sid, seq-1)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(events, func(ev event.Event) bool { return ev.Seq == seq && ev.Type == event.Checkpoint })
	if i < 0 {
		return errors.New("there is no checkpoint there")
	}
	var cp event.CheckpointData
	json.Unmarshal(events[i].Data, &cp)
	// A compaction or an earlier rewind after the checkpoint replaced the
	// history, so the checkpoint's length no longer points into it.
	replaced := slices.ContainsFunc(events[i+1:], func(ev event.Event) bool {
		var h struct {
			Reset bool `json:"reset"`
		}
		return ev.Type == event.HistoryMessage && json.Unmarshal(ev.Data, &h) == nil && h.Reset
	})
	r, err := e.runtimeFor(ctx, sid)
	if err != nil {
		return err
	}
	// The session counts as busy while files are restored, so a prompt
	// sent meanwhile waits in the queue and starts afterwards.
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return errStopped
	}
	if r.busy {
		r.mu.Unlock()
		return errors.New("stop the running turn before rewinding")
	}
	r.busy, r.done = true, make(chan struct{})
	r.mu.Unlock()
	defer e.release(r)
	saved, err := e.snapshot(ctx, r.host, sid, "before a rewind")
	if err != nil {
		return fmt.Errorf("save the current files: %w", err)
	}
	if _, err := e.shadowGit(ctx, r.host, sid, checkpointTimeout, "read-tree", "--reset", "-u", cp.Commit); err != nil {
		return fmt.Errorf("restore the files: %w", err)
	}
	d := event.RewindData{CheckpointSeq: seq, Commit: cp.Commit, Saved: saved}
	if h, ok := r.hs.(History); ok && !replaced && cp.HistoryLen <= h.HistoryLen() {
		h.TruncateHistory(cp.HistoryLen)
		d.Context = true
	}
	e.emit(sid, event.Rewind, d)
	e.gitChanged(sid)
	return nil
}
