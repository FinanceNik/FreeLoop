package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/skill"
)

// SKL-007: a skill created from the template is discovered; a skill
// without a description is flagged; a test run shows whether the model
// picked the skill for a sample prompt.
func TestSkillAuthoring(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	path, err := e.CreateSkill("release-notes", "Write release notes from the git log since the last tag.")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "name: release-notes") || path != e.SkillFile("release-notes") {
		t.Errorf("SKILL.md at %s:\n%s", path, b)
	}
	if _, err := e.CreateSkill("release-notes", "again"); err == nil {
		t.Error("a second skill with the same name was created")
	}
	if _, err := e.CreateSkill("no-desc", "  "); err == nil || !strings.Contains(err.Error(), "description") {
		t.Errorf("a skill without a description: %v", err)
	}

	// A hand-written skill without a description is an issue.
	bad := filepath.Join(e.dataDir, SkillsDir, "half-done")
	os.MkdirAll(bad, 0o700)
	os.WriteFile(filepath.Join(bad, skill.FileName), []byte("---\nname: half-done\n---\nSteps.\n"), 0o600)
	e.forgetSkills()
	list, _ := e.ListSkills(ctx, "")
	if !slices.ContainsFunc(list.Skills, func(s SkillInfo) bool { return s.Name == "release-notes" }) {
		t.Errorf("skills = %+v", list.Skills)
	}
	if !slices.ContainsFunc(list.Issues, func(i skill.Issue) bool { return strings.Contains(i.Message, "description is required") }) {
		t.Errorf("issues = %+v", list.Issues)
	}

	withScript(e, "picks", scripted.Step{Calls: []scripted.Call{{Name: "load_skill", Input: map[string]any{"name": "release-notes"}}}})
	withScript(e, "ignores", scripted.Step{Text: "I will just write them."})
	res, err := e.TestSkill(ctx, SkillTestRequest{Name: "release-notes", ProviderID: "picks", Model: "m", Prompt: "Draft the notes for 1.4"})
	if err != nil || !res.Picked || res.Loaded != "release-notes" {
		t.Errorf("a model that loads it = %+v, %v", res, err)
	}
	res, err = e.TestSkill(ctx, SkillTestRequest{Name: "release-notes", ProviderID: "ignores", Model: "m", Prompt: "Draft the notes for 1.4"})
	if err != nil || res.Picked || res.Reply != "I will just write them." {
		t.Errorf("a model that does not = %+v, %v", res, err)
	}
}
