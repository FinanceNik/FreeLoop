package engine

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// Skills from git (SKL-006): a repository URL, optionally with #ref (a
// branch, tag or commit), is a skill source like a folder. It is cloned
// into a temporary folder in the data folder, pinned to the commit the ref
// names, read, and removed; the commit is recorded with the skill, so
// "check for updates" compares the ref's latest commit with it and shows
// the diff, and each install keeps the version it replaced for a rollback.

// maxSkillDiff caps the text diff a preview shows.
const maxSkillDiff = 32 << 10

// gitSource is a skill repository and the ref and commit it was read at.
type gitSource struct {
	URL, Ref, Commit string
}

// parseGitSource recognises a git URL, with an optional #ref.
func parseGitSource(src string) (gitSource, bool) {
	url, ref, _ := strings.Cut(strings.TrimSpace(src), "#")
	for _, p := range []string{"https://", "http://", "ssh://", "git://", "file://", "git@"} {
		if strings.HasPrefix(url, p) {
			return gitSource{URL: url, Ref: ref}, true
		}
	}
	return gitSource{}, false
}

// gitProtocols are the transports a skill repository may use; no others,
// so a URL cannot make git run a helper program.
var gitProtocols = []string{"-c", "protocol.allow=never", "-c", "protocol.https.allow=always", "-c", "protocol.http.allow=always",
	"-c", "protocol.ssh.allow=always", "-c", "protocol.git.allow=always", "-c", "protocol.file.allow=always"}

// readGit clones a skill repository at its ref and reads its skills.
func (e *Engine) readGit(g gitSource) ([]bundle, error) {
	if netguard.LocalOnly() && !strings.HasPrefix(g.URL, "file://") {
		return nil, errors.New("local-only mode is on, so skills cannot be fetched from a remote repository")
	}
	dir, err := config.NewSkillClone(e.dataDir)
	if err != nil {
		return nil, err
	}
	defer config.RemoveSkillClone(e.dataDir, dir)
	env, err := e.localEnv(filepath.Dir(dir), nil)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	clone := append(append([]string{}, gitProtocols...), "clone", "--quiet", "--no-checkout", "--", g.URL, dir)
	if res, err := gitRun(ctx, env, 4*time.Minute, clone...); err != nil || res.ExitCode != 0 {
		return nil, fmt.Errorf("clone %s: %s", g.URL, cmp.Or(lastLine(res.Output), fmt.Sprint(err)))
	}
	ref := cmp.Or(g.Ref, "HEAD")
	commit, err := gitOK(ctx, env, "-C", dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		if commit, err = gitOK(ctx, env, "-C", dir, "rev-parse", "--verify", "--quiet", "origin/"+ref+"^{commit}"); err != nil {
			return nil, fmt.Errorf("the repository has no branch, tag or commit %q", g.Ref)
		}
	}
	g.Commit = strings.TrimSpace(commit)
	if _, err := gitOK(ctx, env, "-C", dir, "reset", "--quiet", "--hard", g.Commit); err != nil {
		return nil, fmt.Errorf("check out %s: %w", g.Commit, err)
	}
	files := map[string][]byte{}
	ro, err := readOnlyEnv(dir)
	if err != nil {
		return nil, err
	}
	if err := collect(ro.FS(), ".", files); err != nil {
		return nil, err
	}
	bundles, err := group(files, g.URL)
	if err != nil {
		return nil, err
	}
	for i := range bundles {
		bundles[i].git = &g
		bundles[i].cand.Source = fmt.Sprintf("%s at %.12s", g.URL, g.Commit)
	}
	return bundles, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// installedFromGit returns where a store skill came from, when that is a
// git repository.
func (e *Engine) installedFromGit(name string) (gitSource, error) {
	prov, ok := config.ReadSkillProvenance(e.dataDir, name)
	if !ok || prov.Commit == "" {
		return gitSource{}, fmt.Errorf("%s was not installed from a git repository", name)
	}
	return gitSource{URL: prov.Source, Ref: prov.Ref, Commit: prov.Commit}, nil
}

// CheckSkillUpdate reads a git-installed skill's ref again and compares it
// with the installed copy; the candidate's status is same when nothing
// changed, else changed with the files and diff.
func (e *Engine) CheckSkillUpdate(ctx context.Context, name string) (SkillCandidate, error) {
	g, err := e.installedFromGit(name)
	if err != nil {
		return SkillCandidate{}, err
	}
	cands, err := e.PreviewSkills(ctx, gitSourceString(g))
	if err != nil {
		return SkillCandidate{}, err
	}
	for _, c := range cands {
		if c.Name == name {
			return c, nil
		}
	}
	return SkillCandidate{}, fmt.Errorf("the repository no longer has a skill named %s", name)
}

// UpdateSkill installs the latest commit of a git-installed skill's ref;
// the version it replaces is kept for RollbackSkill.
func (e *Engine) UpdateSkill(ctx context.Context, name string) error {
	g, err := e.installedFromGit(name)
	if err != nil {
		return err
	}
	_, err = e.InstallSkills(ctx, gitSourceString(g), []string{name})
	return err
}

// RollbackSkill puts back the version of a store skill its last install
// replaced; doing it again undoes it.
func (e *Engine) RollbackSkill(name string) error {
	if err := config.RollbackSkill(e.dataDir, name); err != nil {
		return err
	}
	e.forgetSkills()
	return nil
}

func gitSourceString(g gitSource) string {
	if g.Ref == "" {
		return g.URL
	}
	return g.URL + "#" + g.Ref
}
