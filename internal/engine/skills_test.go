package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, body := range files {
		full := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(body), 0o644)
	}
}

func lastToolResult(req provider.ChatRequest) string {
	m := req.Messages[len(req.Messages)-1]
	for _, p := range m.Parts {
		if p.Type == provider.PartToolResult {
			return p.Text
		}
	}
	return ""
}

// SKL-001, SKL-002, SKL-004: skills from the app store and a trusted
// project are listed by name and description only, load_skill reads them
// and their files on demand, and switches at every scope take effect.
func TestSkills(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	writeFiles(t, filepath.Join(dir, SkillsDir), map[string]string{
		"pdf-tools/SKILL.md":           "---\nname: pdf-tools\ndescription: Fill and merge PDF forms.\nallowed-tools: Read, Bash\n---\n# PDF tools\nRun scripts/fill.py with python.\n",
		"pdf-tools/scripts/fill.py":    "print('fill')\n",
		"pdf-tools/references/spec.md": "The form spec.",
		"shared/SKILL.md":              "---\nname: shared\ndescription: From the app store.\n---\napp body\n",
	})
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".claude/skills/shared/SKILL.md": "---\nname: shared\ndescription: From the project.\n---\nproject body\n",
		".agents/skills/lint/SKILL.md":   "---\nname: lint\ndescription: Lint the code.\n---\nRun the linter.\n",
	})
	prov := scripted.New("p",
		scripted.Step{Calls: []scripted.Call{{Name: "load_skill", Input: map[string]any{"name": "pdf-tools"}}}},
		scripted.Step{Calls: []scripted.Call{{Name: "load_skill", Input: map[string]any{"name": "pdf-tools", "file": "references/spec.md"}}}},
		scripted.Step{Calls: []scripted.Call{{Name: "load_skill", Input: map[string]any{"name": "pdf-tools", "file": "../../config.yaml"}}}},
		scripted.Step{Text: "done"},
	)
	e.mu.Lock()
	e.providers.built["p"] = &configuredProvider{cfg: config.Provider{ID: "p", Name: "p", Type: "scripted"}, p: prov}
	e.mu.Unlock()

	// Untrusted: the app store's skills only.
	list, _ := e.ListSkills(ctx, root)
	if len(list.Skills) != 2 || list.ProjectID != "" {
		t.Fatalf("untrusted list = %+v", list)
	}
	e.SetProjectTrust(ctx, root, true)
	list, _ = e.ListSkills(ctx, root)
	byName := map[string]SkillInfo{}
	for _, s := range list.Skills {
		byName[s.Name] = s
	}
	if len(list.Skills) != 3 || byName["shared"].Source != ".claude" || byName["lint"].Source != ".agents" || byName["pdf-tools"].Source != "app" {
		t.Fatalf("trusted list = %+v", list)
	}

	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "p", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, ses.ID, "fill the form")
	waitIdle(t, ch, ses.ID)
	reqs := prov.Requests()
	sys := reqs[0].System
	for _, want := range []string{"# Skills", "- pdf-tools: Fill and merge PDF forms.", "- shared: From the project.", "- lint: Lint the code."} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	if strings.Contains(sys, "Run scripts/fill.py") {
		t.Error("a skill body is in the system prompt")
	}
	if !slices.ContainsFunc(reqs[0].Tools, func(s provider.ToolSpec) bool { return s.Name == "load_skill" }) {
		t.Error("load_skill not offered")
	}
	if got := lastToolResult(reqs[1]); !strings.Contains(got, "Run scripts/fill.py") || !strings.Contains(got, "scripts/fill.py") || !strings.Contains(got, filepath.Join(dir, SkillsDir, "pdf-tools")) {
		t.Errorf("load_skill result = %q", got)
	}
	if got := lastToolResult(reqs[2]); got != "The form spec." {
		t.Errorf("load_skill file = %q", got)
	}
	if got := lastToolResult(reqs[3]); !strings.Contains(got, "not a file inside the skill") {
		t.Errorf("escaping the skill folder = %q", got)
	}

	// SKL-004: each enabled skill adds one line of name and capped
	// description, well under 100 tokens (about 400 characters).
	long := strings.Repeat("word ", 190)
	writeFiles(t, filepath.Join(dir, SkillsDir), map[string]string{"long/SKILL.md": "---\nname: long\ndescription: " + long + "\n---\nbody\n"})
	e.forgetSkills()
	for _, sk := range (sessionSkills{e: e, projectID: ses.ProjectID, root: root}).Enabled() {
		if sk.Name == "long" && len(sk.Description) < 900 {
			t.Fatal("the test skill's description is short")
		}
	}
	e.Prompt(ctx, ses.ID, "again")
	waitIdle(t, ch, ses.ID)
	reqs = prov.Requests()
	found := false
	for _, line := range strings.Split(reqs[len(reqs)-1].System, "\n") {
		if strings.HasPrefix(line, "- long: ") {
			found = true
			if len(line) > 300 {
				t.Errorf("index line is %d characters", len(line))
			}
		}
	}
	if !found {
		t.Error("the long skill is not in the index")
	}

	// Switches: globally, per project, per profile.
	if err := e.SetSkillEnabled(ctx, "pdf-tools", "", false); err != nil {
		t.Fatal(err)
	}
	if err := e.SetSkillEnabled(ctx, "lint", ses.ProjectID, false); err != nil {
		t.Fatal(err)
	}
	src := sessionSkills{e: e, projectID: ses.ProjectID, root: root, profileID: ""}
	names := func() []string {
		var out []string
		for _, s := range src.Enabled() {
			out = append(out, s.Name)
		}
		return out
	}
	if got := names(); !slices.Equal(got, []string{"long", "shared"}) {
		t.Errorf("after switches = %v", got)
	}
	if _, err := src.Load("pdf-tools", ""); err == nil {
		t.Error("a disabled skill loaded")
	}
	e.SetSkillEnabled(ctx, "pdf-tools", "", true)
	e.SaveProfile(config.Profile{ID: "only-pdf", Name: "Only PDF", Skills: []string{"pdf-tools"}})
	src.profileID = "only-pdf"
	if got := names(); !slices.Equal(got, []string{"pdf-tools"}) {
		t.Errorf("profile allowlist = %v", got)
	}

	// Rules match skills by name.
	r := permission.Rule{Effect: permission.Deny, Tool: "skill:pdf-tools"}
	if !r.Matches(permission.Request{Tool: "skill:pdf-tools", Kind: permission.KindRead}, root) {
		t.Error("skill rule does not match")
	}
}
