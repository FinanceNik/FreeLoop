package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(body))
	}
	w.Close()
	return buf.Bytes()
}

// SKL-003 and SKL-005: skills are imported from ~/.claude/skills with
// provenance, re-importing shows what changed and never duplicates, the
// source stays untouched, and zips install after the same preview.
func TestImportAndInstallSkills(t *testing.T) {
	ctx := context.Background()
	home, dir := t.TempDir(), t.TempDir()
	e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: home, Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	src := e.ClaudeSkillsDir()
	writeFiles(t, src, map[string]string{
		"commit-msg/SKILL.md":     "---\nname: commit-msg\ndescription: Write commit messages.\nmodel: sonnet\n---\nBody v1\n",
		"commit-msg/scripts/x.sh": "#!/bin/sh\necho hi\n",
		"commit-msg/references/a": "a",
		"broken/SKILL.md":         "---\nname: wrong\ndescription: x\n---\n",
		"not-a-skill/readme.txt":  "x",
	})
	before, _ := os.ReadFile(filepath.Join(src, "commit-msg", "SKILL.md"))

	cands, err := e.PreviewSkills(ctx, "~/.claude/skills")
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]SkillCandidate{}
	for _, c := range cands {
		status[c.Name] = c
	}
	if len(cands) != 2 || status["commit-msg"].Status != "new" || status["broken"].Status != "invalid" ||
		!slices.Equal(status["commit-msg"].Scripts, []string{"scripts/x.sh"}) {
		t.Fatalf("preview = %+v", cands)
	}
	if done, err := e.InstallSkills(ctx, src, []string{"commit-msg"}); err != nil || !slices.Equal(done, []string{"commit-msg"}) {
		t.Fatalf("install = %v, %v", done, err)
	}
	if _, err := e.InstallSkills(ctx, src, []string{"broken"}); err == nil {
		t.Error("an invalid skill installed")
	}
	installed := filepath.Join(dir, SkillsDir, "commit-msg")
	if fi, err := os.Stat(filepath.Join(installed, "scripts", "x.sh")); err != nil || goruntime.GOOS != "windows" && fi.Mode()&0o100 == 0 {
		t.Errorf("script = %v, %v", fi, err)
	}
	prov, ok := config.ReadSkillProvenance(dir, "commit-msg")
	if !ok || prov.Source != src || prov.ImportedAt.IsZero() || prov.Digest == "" {
		t.Errorf("provenance = %+v, %v", prov, ok)
	}
	if after, _ := os.ReadFile(filepath.Join(src, "commit-msg", "SKILL.md")); !bytes.Equal(before, after) {
		t.Error("the source changed")
	}
	list, _ := e.ListSkills(ctx, "")
	if len(list.Skills) != 1 || list.Skills[0].Name != "commit-msg" {
		t.Errorf("store = %+v", list)
	}

	// Re-import: unchanged, then changed with a file-level diff.
	cands, _ = e.PreviewSkills(ctx, src)
	for _, c := range cands {
		if c.Name == "commit-msg" && (c.Status != "same" || c.Installed == nil) {
			t.Errorf("unchanged re-import = %+v", c)
		}
	}
	if done, _ := e.InstallSkills(ctx, src, []string{"commit-msg"}); len(done) != 0 {
		t.Errorf("an identical skill was rewritten: %v", done)
	}
	writeFiles(t, src, map[string]string{"commit-msg/SKILL.md": "---\nname: commit-msg\ndescription: Write commit messages.\n---\nBody v2\n", "commit-msg/new.md": "n"})
	os.Remove(filepath.Join(src, "commit-msg", "references", "a"))
	cands, _ = e.PreviewSkills(ctx, filepath.Join(src, "commit-msg"))
	if len(cands) != 1 || cands[0].Status != "changed" || !slices.Equal(cands[0].Changes, []string{"~ SKILL.md", "+ new.md", "- references/a"}) {
		t.Fatalf("changed preview = %+v", cands)
	}
	e.InstallSkills(ctx, filepath.Join(src, "commit-msg"), []string{"commit-msg"})
	if _, err := os.Stat(filepath.Join(installed, "references", "a")); !os.IsNotExist(err) {
		t.Error("a file removed upstream stayed in the store")
	}
	// One skill, plus the version the update replaced kept for a rollback
	// (SKL-006); no temporary folders left behind.
	entries, _ := os.ReadDir(filepath.Join(dir, SkillsDir))
	var names []string
	for _, en := range entries {
		names = append(names, en.Name())
	}
	if strings.Join(names, ",") != ".previous,commit-msg" {
		t.Errorf("store holds %v, want the skill and .previous (no duplicates or leftovers)", names)
	}

	// Zips: a skill at the top, named by its frontmatter; paths that leave
	// the archive are refused.
	zdir := t.TempDir()
	os.WriteFile(filepath.Join(zdir, "download.zip"), zipOf(t, map[string]string{"SKILL.md": "---\nname: zipped\ndescription: From a zip.\n---\nx\n", "scripts/run.py": "print(1)"}), 0o644)
	os.WriteFile(filepath.Join(zdir, "evil.zip"), zipOf(t, map[string]string{"s/SKILL.md": "---\nname: s\ndescription: x\n---\n", "../../outside": "x"}), 0o644)
	cands, err = e.PreviewSkills(ctx, filepath.Join(zdir, "download.zip"))
	if err != nil || len(cands) != 1 || cands[0].Name != "zipped" || cands[0].Status != "new" {
		t.Fatalf("zip preview = %+v, %v", cands, err)
	}
	if _, err := e.InstallSkills(ctx, filepath.Join(zdir, "download.zip"), []string{"zipped"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.PreviewSkills(ctx, filepath.Join(zdir, "evil.zip")); err == nil {
		t.Error("a zip escaping its folder was accepted")
	}
	if err := e.RemoveSkill("zipped"); err != nil {
		t.Fatal(err)
	}
	if list, _ := e.ListSkills(ctx, ""); len(list.Skills) != 1 {
		t.Errorf("after remove = %+v", list.Skills)
	}
}
