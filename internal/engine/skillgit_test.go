package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// SKL-006: a skill installs from a git URL pinned to a commit; "check for
// updates" shows the diff of a newer commit; update and rollback each leave
// the skill loadable; a ref pins an older commit; local-only mode refuses
// remote repositories.
func TestSkillsFromGit(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	gitIn(t, repo, "config", "user.name", "t")
	gitIn(t, repo, "config", "user.email", "t@t")
	write := func(body string) string {
		os.MkdirAll(filepath.Join(repo, "greet"), 0o755)
		os.WriteFile(filepath.Join(repo, "greet", "SKILL.md"), []byte("---\nname: greet\ndescription: Greets people.\n---\n"+body), 0o644)
		gitIn(t, repo, "add", "-A")
		gitIn(t, repo, "commit", "-qm", body)
		return strings.TrimSpace(gitIn(t, repo, "rev-parse", "HEAD"))
	}
	first := write("Say hello.\n")
	url := "file://" + repo

	e := openTest(t, t.TempDir())
	defer e.Close()
	if done, err := e.InstallSkills(ctx, url, []string{"greet"}); err != nil || len(done) != 1 {
		t.Fatalf("install = %v, %v", done, err)
	}
	body := func() string {
		b, _ := os.ReadFile(filepath.Join(e.dataDir, SkillsDir, "greet", "SKILL.md"))
		return string(b)
	}
	loadable := func() bool {
		list, _ := e.ListSkills(ctx, "")
		return slices.ContainsFunc(list.Skills, func(s SkillInfo) bool { return s.Name == "greet" })
	}
	list, _ := e.ListSkills(ctx, "")
	if i := slices.IndexFunc(list.Skills, func(s SkillInfo) bool { return s.Name == "greet" }); i < 0 || list.Skills[i].Installed == nil || list.Skills[i].Installed.Commit != first {
		t.Fatalf("skills = %+v", list.Skills)
	}

	write("Say hello and ask how they are.\n")
	c, err := e.CheckSkillUpdate(ctx, "greet")
	if err != nil || c.Status != "changed" || !strings.Contains(c.Diff, "+Say hello and ask how they are.") || !strings.Contains(c.Diff, "-Say hello.") {
		t.Fatalf("update check = %+v, %v", c, err)
	}
	if err := e.UpdateSkill(ctx, "greet"); err != nil || !strings.Contains(body(), "ask how they are") || !loadable() {
		t.Fatalf("update: %v, body %q", err, body())
	}
	if c, _ := e.CheckSkillUpdate(ctx, "greet"); c.Status != "same" {
		t.Errorf("after the update the check says %s", c.Status)
	}
	if err := e.RollbackSkill("greet"); err != nil || strings.Contains(body(), "ask how they are") || !loadable() {
		t.Fatalf("rollback: %v, body %q", err, body())
	}
	if err := e.RollbackSkill("greet"); err != nil || !strings.Contains(body(), "ask how they are") {
		t.Errorf("undoing the rollback: %v, body %q", err, body())
	}

	// A ref pins an older commit.
	e.RemoveSkill("greet")
	if _, err := e.InstallSkills(ctx, url+"#"+first, []string{"greet"}); err != nil || strings.Contains(body(), "ask how they are") {
		t.Errorf("pinned install: %v, body %q", err, body())
	}
	if _, err := e.PreviewSkills(ctx, url+"#no-such-branch"); err == nil {
		t.Error("a missing ref was accepted")
	}

	if left, _ := os.ReadDir(filepath.Join(e.dataDir, SkillsDir, ".clones")); len(left) != 0 {
		t.Errorf("checkouts left behind: %d", len(left))
	}

	netguard.SetLocalOnly(true)
	defer netguard.SetLocalOnly(false)
	if _, err := e.PreviewSkills(ctx, "https://example.com/skills.git"); err == nil || !strings.Contains(err.Error(), "local-only") {
		t.Errorf("a remote repository in local-only mode: %v", err)
	}
}
