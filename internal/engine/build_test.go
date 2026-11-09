package engine

import (
	"runtime/debug"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// A build says which commit it is from, so two machines on the same version can tell
// whether they run the same code: the one the build task linked in, else Go's own
// record, with a mark when the files had changes that were not committed.
func TestBuildSaysItsCommit(t *testing.T) {
	vcs := func(rev, modified string) []debug.BuildSetting {
		return []debug.BuildSetting{{Key: "vcs.revision", Value: rev}, {Key: "vcs.modified", Value: modified}}
	}
	for _, c := range []struct {
		name, linked string
		settings     []debug.BuildSetting
		label        string
		commit       string
		dirty        bool
	}{
		{"linked in", "a59ce34", nil, "0.1.0-beta.1 · a59ce34", "a59ce34", false},
		{"linked in, uncommitted changes", "a59ce34+dirty", nil, "0.1.0-beta.1 · a59ce34+dirty", "a59ce34", true},
		{"the link wins over Go's record", "a59ce34", vcs("1234567890abcdef", "false"), "0.1.0-beta.1 · a59ce34", "a59ce34", false},
		{"Go's record", "", vcs("a59ce34f00dbabe00000000000000000000000", "false"), "0.1.0-beta.1 · a59ce34", "a59ce34", false},
		{"Go's record with changes", "", vcs("a59ce34f00dbabe00000000000000000000000", "true"), "0.1.0-beta.1 · a59ce34+dirty", "a59ce34", true},
		{"nothing known", "", nil, "0.1.0-beta.1", "", false},
		{"a record too short to be a commit", "", vcs("abc", "false"), "0.1.0-beta.1", "", false},
		{"only the mark", "+dirty", nil, "0.1.0-beta.1", "", false},
	} {
		b := buildInfo("0.1.0-beta.1", c.linked, c.settings)
		stamp := ""
		if c.commit != "" {
			stamp = strings.TrimPrefix(c.label, "0.1.0-beta.1 · ")
		}
		if b.Label != c.label || b.Commit != c.commit || b.Dirty != c.dirty || b.Stamp != stamp || b.Version != "0.1.0-beta.1" {
			t.Errorf("%s: %+v, want label %q commit %q dirty %v", c.name, b, c.label, c.commit, c.dirty)
		}
	}
}

// The problem report starts with the build, so an issue says which one it is about.
func TestProblemReportNamesTheBuild(t *testing.T) {
	old := BuildCommit
	BuildCommit = "a59ce34"
	t.Cleanup(func() { BuildCommit = old })
	e := openBinding(t, t.TempDir(), secrets.NewMemory())
	text, err := e.ProblemReport(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if first, _, _ := strings.Cut(text, "\n"); !strings.HasPrefix(first, "Freeloop "+AppVersion+" · a59ce34 (") {
		t.Errorf("first line: %q", first)
	}
}
