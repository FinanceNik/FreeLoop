package backlog

import (
	"strings"
	"testing"
)

func issue(path, fm string) Issue {
	return ParseIssue(path, []byte("---\n"+fm+"\n---\n\n## What\nSomething.\n"))
}

func TestParseIssue(t *testing.T) {
	is := issue("backlog/issues/002-search.md", "title: Search drafts\nstatus: open\ntype: afk\nblockedBy: [001-store.md, \"#003-x\"]\nprd: drafts.md")
	if is.ID != "002-search" || is.Title != "Search drafts" || is.Status != StatusOpen || is.Type != TypeAFK || is.PRD != "drafts" || is.Problem != "" {
		t.Errorf("%+v", is)
	}
	if strings.Join(is.BlockedBy, ",") != "001-store,003-x" {
		t.Errorf("blockedBy = %v", is.BlockedBy)
	}
	// One blocker as a string, done in another word, a title from the heading.
	is = ParseIssue("x/003-y.md", []byte("---\nstatus: Completed\nblockedBy: 002-search\n---\n# The heading\n"))
	if is.Status != StatusDone || is.Title != "The heading" || len(is.BlockedBy) != 1 || is.Type != TypeHuman {
		t.Errorf("%+v", is)
	}
	// What cannot be read never runs unattended.
	for _, bad := range []string{"no frontmatter at all\n", "---\ntype: afk\nstatus: [\n---\n", "---\ntype: afk\nstatus: maybe\n---\n", "---\ntype: robot\n---\n"} {
		if is := ParseIssue("i/004.md", []byte(bad)); is.Problem == "" || is.Type != TypeHuman {
			t.Errorf("%q: %+v", bad, is)
		}
	}
	// Windows line ends and an empty frontmatter.
	if is := ParseIssue("i/005.md", []byte("---\r\ntitle: CRLF\r\ntype: afk\r\n---\r\nbody\r\n")); is.Title != "CRLF" || is.Type != TypeAFK {
		t.Errorf("crlf: %+v", is)
	}
	if b := Body([]byte("---\ntitle: x\n---\nThe body.\n")); b != "The body.\n" {
		t.Errorf("body %q", b)
	}
}

func TestAnalyzeOrdersAndBlocks(t *testing.T) {
	items := Analyze([]Issue{
		issue("b/issues/002-two.md", "type: afk\nblockedBy: [001-one]"),
		issue("b/issues/001-one.md", "type: afk"),
		issue("b/issues/003-three.md", "type: human\nblockedBy: [001-one]"),
		issue("b/issues/004-four.md", "type: afk\nblockedBy: [404-missing]"),
		issue("b/issues/005-a.md", "type: afk\nblockedBy: [006-b]"),
		issue("b/issues/006-b.md", "type: afk\nblockedBy: [005-a]"),
	})
	state := map[string]Item{}
	for _, it := range items {
		state[it.ID] = it
	}
	if items[0].ID != "001-one" || state["001-one"].State != Ready || strings.Join(state["001-one"].Blocks, ",") != "002-two,003-three" {
		t.Errorf("001: %+v", state["001-one"])
	}
	if state["002-two"].State != Blocked || strings.Join(state["002-two"].WaitsOn, ",") != "001-one" {
		t.Errorf("002: %+v", state["002-two"])
	}
	if state["004-four"].State != Blocked {
		t.Errorf("a missing blocker blocks: %+v", state["004-four"])
	}
	if !state["005-a"].Cycle || !state["006-b"].Cycle || state["005-a"].State != Blocked {
		t.Errorf("cycle: %+v %+v", state["005-a"], state["006-b"])
	}
	next, ok := NextAFK(items, nil)
	if !ok || next.ID != "001-one" {
		t.Fatalf("next = %+v", next)
	}
	if _, ok := NextAFK(items, func(id string) bool { return id == "001-one" }); ok {
		t.Error("nothing else is ready and AFK")
	}

	// Once the first is done, the second is ready; the human one is never picked.
	items = Analyze([]Issue{
		issue("b/issues/001-one.md", "type: afk\nstatus: done\nprd: p"),
		issue("b/issues/002-two.md", "type: afk\nblockedBy: [001-one]\nprd: p"),
		issue("b/issues/003-three.md", "type: human\nblockedBy: [001-one]"),
	})
	if next, ok := NextAFK(items, nil); !ok || next.ID != "002-two" {
		t.Errorf("next = %+v", next)
	}
	sum := Summary(items)
	if !strings.Contains(sum, "- b/issues/001-one.md [done, afk]") || !strings.Contains(sum, "[ready, human]") {
		t.Errorf("summary:\n%s", sum)
	}
	prds := []PRD{{ID: "p"}, {ID: "q"}, {ID: "r", Status: StatusDone}}
	if f := Finished(prds, items); len(f) != 1 || f[0].ID != "r" {
		t.Errorf("finished = %+v", f)
	}
	items[1].State = Done
	if f := Finished(prds, items); len(f) != 2 || f[0].ID != "p" {
		t.Errorf("finished = %+v", f)
	}
}
