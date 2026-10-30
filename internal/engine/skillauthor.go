package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/session"
	"github.com/FinanceNik/Freeloop/internal/skill"
)

// Skill authoring (SKL-007): a new skill from a template in the app's
// skill store, opened in the user's editor, checked by the same parser that
// discovers skills (a missing description is an issue on the Skills page),
// and test-run: one model call with the skills index and load_skill tool a
// session gets, to see whether the model picks the skill for a prompt.

const skillTemplate = `---
name: %s
description: %s
---

# %s

Say what this skill is for and when to use it, then the steps to follow.

## Steps

1. …

## Notes

Files next to this one (scripts, references) can be read with load_skill and run with the shell tool.
`

// CreateSkill makes a skill from the template in the app's store and
// returns its SKILL.md.
func (e *Engine) CreateSkill(name, description string) (string, error) {
	name, description = strings.TrimSpace(name), strings.Join(strings.Fields(description), " ")
	switch {
	case !skill.ValidName(name):
		return "", fmt.Errorf("the name %q must be lowercase letters, digits and single hyphens, at most 64 characters", name)
	case description == "":
		return "", errors.New("a skill needs a description: models pick skills by it")
	case len(description) > 1024:
		return "", errors.New("the description is longer than 1024 characters")
	}
	dir := filepath.Join(e.dataDir, SkillsDir, name)
	if environment.IsDir(dir) {
		return "", fmt.Errorf("a skill named %s exists", name)
	}
	body := fmt.Sprintf(skillTemplate, name, description, name)
	if err := config.InstallSkill(e.dataDir, name, map[string][]byte{skill.FileName: []byte(body)},
		config.SkillProvenance{Source: "created in Freeloop", ImportedAt: time.Now().UTC()}); err != nil {
		return "", err
	}
	e.forgetSkills()
	return filepath.Join(dir, skill.FileName), nil
}

// SkillFile returns the SKILL.md of a skill in the app's store, for the
// editor; "" for a skill elsewhere.
func (e *Engine) SkillFile(name string) string {
	if !skill.ValidName(name) {
		return ""
	}
	f := filepath.Join(e.dataDir, SkillsDir, name, skill.FileName)
	if environment.IsDir(filepath.Dir(f)) {
		return f
	}
	return ""
}

// SkillTestRequest asks whether a model picks a skill for a prompt.
type SkillTestRequest struct {
	Name       string `json:"name"`
	ProviderID string `json:"providerId"`
	Model      string `json:"model"`
	Prompt     string `json:"prompt"`
	// Root is the project whose skills are listed alongside, if any.
	Root string `json:"root"`
}

// SkillTest is a test run's result: whether the model loaded the skill,
// what it loaded instead, and what it said.
type SkillTest struct {
	Picked bool   `json:"picked"`
	Loaded string `json:"loaded"`
	Reply  string `json:"reply"`
}

// TestSkill runs a prompt past a model with the skills a session would list
// (the tested one included even if it is off) and reports whether the model
// called load_skill for it. The call's tokens are not booked to a session.
func (e *Engine) TestSkill(ctx context.Context, req SkillTestRequest) (SkillTest, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return SkillTest{}, errors.New("write a sample prompt")
	}
	var project session.Project
	if req.Root != "" && environment.IsDir(req.Root) {
		if p, err := e.store.ProjectByRoot(ctx, environment.Canonical(req.Root)); err == nil {
			project = p
		}
	}
	off := e.Config().Skills.Disabled
	var entries []harness.SkillEntry
	found := false
	for _, s := range e.discoverSkills(project).skills {
		if s.Name == req.Name {
			found = true
		} else if slices.Contains(off, s.Name) {
			continue
		}
		entries = append(entries, harness.SkillEntry{Name: s.Name, Description: s.Description})
	}
	if !found {
		return SkillTest{}, fmt.Errorf("no skill named %s is found; check the issues on the Skills page", req.Name)
	}
	cp, err := e.provider(req.ProviderID)
	if err != nil {
		return SkillTest{}, err
	}
	model := cmpOr(req.Model, cp.cfg.DefaultModel)
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ch, err := e.limit(cp, "").Stream(cctx, provider.ChatRequest{
		Model:    model,
		System:   "You are a coding agent working in the user's project." + builtin.SkillsIndex(entries),
		Messages: []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{{Type: provider.PartText, Text: req.Prompt}}}},
		Tools:    []provider.ToolSpec{builtin.LoadSkillSpec()},
	})
	if err != nil {
		return SkillTest{}, err
	}
	for ev := range ch {
		switch ev.Type {
		case provider.EventError:
			return SkillTest{}, ev.Err
		case provider.EventDone:
			var res SkillTest
			if ev.Message != nil {
				for _, p := range ev.Message.Parts {
					switch {
					case p.Type == provider.PartText:
						res.Reply += p.Text
					case p.Type == provider.PartToolUse && p.ToolName == "load_skill" && res.Loaded == "":
						var in struct{ Name string }
						json.Unmarshal(p.Input, &in)
						res.Loaded = in.Name
					}
				}
			}
			res.Picked = res.Loaded == req.Name
			return res, nil
		}
	}
	return SkillTest{}, errors.New("the provider stream ended without a response")
}
