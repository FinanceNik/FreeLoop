package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// A model set (WFL-013) names the model and effort for each part of a workflow: its
// planning stages, its AFK issues and their reviews. Sets live in config.yaml, and a
// workflow picks one there, not in its file: a model is named by the user's own
// provider ID, the built-in workflow cannot be edited, and an edit to a file workflow
// would end its approval.

// ModelChoice is one row of a set: a provider's model and an effort. With no model the
// row is not fixed, and the profile's or the session's model stays. Provider is empty
// for a model an external agent names itself.
type ModelChoice struct {
	Provider string `yaml:"provider,omitempty" json:"provider"`
	Model    string `yaml:"model,omitempty" json:"model"`
	Effort   string `yaml:"effort,omitempty" json:"effort"`
}

// Fixed reports whether the row names a model.
func (c ModelChoice) Fixed() bool { return c.Model != "" }

// Value is the row's model as a session's model option takes it: the built-in agent's
// provider and model a line apart, or an external agent's model alone.
func (c ModelChoice) Value() string {
	if c.Provider == "" {
		return c.Model
	}
	return c.Provider + "\n" + c.Model
}

// ModelSet is a named choice of models for a workflow.
type ModelSet struct {
	ID       string      `yaml:"id" json:"id"`
	Name     string      `yaml:"name" json:"name"`
	Planning ModelChoice `yaml:"planning,omitempty" json:"planning"`
	Issues   ModelChoice `yaml:"issues,omitempty" json:"issues"`
	Reviews  ModelChoice `yaml:"reviews,omitempty" json:"reviews"`
}

// Rows are the set's rows by their names in the file.
func (s ModelSet) Rows() map[string]ModelChoice {
	return map[string]ModelChoice{"planning": s.Planning, "issues": s.Issues, "reviews": s.Reviews}
}

var notIDChars = regexp.MustCompile(`[^a-z0-9]+`)

// ModelSetID makes a set's ID from its name.
func ModelSetID(name string) string {
	id := strings.Trim(notIDChars.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if id == "" {
		return "set"
	}
	return id
}

// validateModelSets keeps the sets with a name, an ID of their own and rows that are
// whole: an effort that is one, and none without a model.
func validateModelSets(sets []ModelSet) ([]ModelSet, []Issue) {
	var issues []Issue
	out := sets[:0:0]
	for i, s := range sets {
		bad := ""
		switch {
		case strings.TrimSpace(s.Name) == "":
			bad = "a model set needs a name"
		case s.ID == "" || s.ID != ModelSetID(s.ID):
			bad = fmt.Sprintf("%q is not a model set ID", s.ID)
		case slices.ContainsFunc(out, func(o ModelSet) bool { return o.ID == s.ID }):
			bad = fmt.Sprintf("there are two model sets %q", s.ID)
		}
		for _, name := range []string{"planning", "issues", "reviews"} {
			row := s.Rows()[name]
			switch {
			case bad != "":
			case row.Effort != "" && !slices.Contains(efforts, row.Effort):
				bad = fmt.Sprintf("%s: %q must be one of %s", name, row.Effort, strings.Join(efforts, ", "))
			case row.Effort != "" && !row.Fixed():
				bad = fmt.Sprintf("%s: an effort needs a model", name)
			case strings.ContainsAny(row.Model+row.Provider, "\r\n"):
				bad = fmt.Sprintf("%s: a model is one line", name)
			}
		}
		if bad != "" {
			issues = append(issues, Issue{Field: fmt.Sprintf("modelSets[%d]", i), Message: bad})
			continue
		}
		out = append(out, s)
	}
	return out, issues
}

// validateWorkflowModelSets drops a workflow's set that does not exist.
func validateWorkflowModelSets(m map[string]string, sets []ModelSet) []Issue {
	var issues []Issue
	for wf, id := range m {
		if !slices.ContainsFunc(sets, func(s ModelSet) bool { return s.ID == id }) {
			issues = append(issues, Issue{Field: "workflowModelSets." + wf, Message: fmt.Sprintf("there is no model set %q", id)})
			delete(m, wf)
		}
	}
	return issues
}
