package engine

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// PRV-008: a built-in session's model calls record their model; today's
// estimate uses the price table, including prices added after the calls,
// and a model without a price is reported as unpriced.
func TestUsageCosts(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	p := scripted.New("sp", scripted.Step{Text: "first"}, scripted.Step{Text: "second"})
	e.mu.Lock()
	e.providers.built["sp"] = &configuredProvider{cfg: config.Provider{ID: "sp", Name: "sp", Type: "scripted"}, p: p}
	e.mu.Unlock()
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "sp", Model: "big-1", Mode: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, ses.ID, "one")
	waitIdle(t, ch, ses.ID)
	e.Prompt(ctx, ses.ID, "two")
	waitIdle(t, ch, ses.ID)

	evs, _ := e.Events(ctx, ses.ID, 0, 0)
	var calls int
	for _, ev := range evs {
		if ev.Type == event.UsageUpdate {
			calls++
			if !strings.Contains(string(ev.Data), `"providerId":"sp","model":"big-1"`) {
				t.Errorf("usage = %s", ev.Data)
			}
		}
	}
	if calls != 2 {
		t.Fatalf("usage events = %d", calls)
	}

	days, err := e.Usage(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || len(days[0].Models) != 1 || days[0].Models[0].Priced || days[0].Cost != 0 || days[0].Models[0].InputTokens != 200 {
		t.Fatalf("without prices = %+v", days)
	}

	c := e.Config()
	c.Prices = []config.Price{{ProviderID: "other", Model: "*", Input: 100}, {Model: "big-*", Input: 3, Output: 15}}
	if _, err := e.SaveSettings(c); err != nil {
		t.Fatal(err)
	}
	days, _ = e.Usage(ctx, 30)
	m := days[0].Models[0]
	want := (200*3.0 + float64(m.OutputTokens)*15) / 1e6
	if !m.Priced || math.Abs(days[0].Cost-want) > 1e-12 {
		t.Errorf("with prices = %+v, want %v", days, want)
	}

	// The analytics report has the same tokens per session, with what it
	// groups by and the cost; a deleted session's tokens stay.
	e.SetTags(ctx, ses.ID, []string{"billing"})
	rep, err := e.UsageReport(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rows) != 1 || rep.Until == "" || rep.Since > rep.Until {
		t.Fatalf("report = %+v", rep)
	}
	r := rep.Rows[0]
	if r.SessionID != ses.ID || r.ProjectRoot != ses.ProjectRoot || r.HarnessID != ses.HarnessID || r.InputTokens != 200 || r.Calls != 2 ||
		!r.Priced || math.Abs(r.Cost-want) > 1e-12 || len(r.Tags) != 1 || r.Deleted ||
		math.Abs(r.CostInput+r.CostOutput+r.CostCacheRead+r.CostCacheWrite-r.Cost) > 1e-12 || math.Abs(r.CostInput-200*3.0/1e6) > 1e-12 {
		t.Errorf("row = %+v", r)
	}
	out := filepath.Join(t.TempDir(), "usage.csv")
	e.RenameSession(ctx, ses.ID, "=cmd|' /C calc'!A0")
	if err := e.ExportUsage(ctx, 7, out); err != nil {
		t.Fatal(err)
	}
	csvText, _ := os.ReadFile(out)
	if lines := strings.Split(strings.TrimSpace(string(csvText)), "\n"); len(lines) != 2 || !strings.HasPrefix(lines[0], "day,session,title") ||
		!strings.Contains(lines[1], ",'=cmd|") || !strings.Contains(lines[1], ",200,") {
		t.Errorf("csv = %s", csvText)
	}
	if err := e.DeleteSession(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	rep, _ = e.UsageReport(ctx, 7)
	if len(rep.Rows) != 1 || !rep.Rows[0].Deleted || rep.Rows[0].InputTokens != 200 {
		t.Errorf("after delete = %+v", rep.Rows)
	}

	c.Prices = []config.Price{{Model: "x", Input: -1}}
	if _, err := e.SaveSettings(c); err == nil {
		t.Error("a negative price was saved")
	}
}

// PRV-008: the app's own calls are spent too. A commit message suggestion records a
// usage event against its session, and a skill's test, which has no session, books
// its tokens to the day; the Usage page's totals count both.
func TestAppCallsRecordUsage(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	e := openTest(t, t.TempDir())
	defer e.Close()
	withScript(e, "msg", scripted.Step{Text: "Change a"})
	if _, err := e.SetProjectTrust(ctx, repo, true); err != nil {
		t.Fatal(err)
	}
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "msg", Model: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed\n"), 0o644)
	if _, err := e.SuggestCommitMessage(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	evs, _ := e.Events(ctx, ses.ID, 0, 0)
	var got []event.UsageData
	for _, ev := range evs {
		if ev.Type == event.UsageUpdate {
			var d event.UsageData
			json.Unmarshal(ev.Data, &d)
			got = append(got, d)
		}
	}
	if len(got) != 1 || got[0].ProviderID != "msg" || got[0].Model != "m1" || got[0].InputTokens == 0 || got[0].OutputTokens == 0 || got[0].Partial {
		t.Fatalf("the suggestion's usage = %+v", got)
	}

	if _, err := e.CreateSkill("release-notes", "Write release notes."); err != nil {
		t.Fatal(err)
	}
	withScript(e, "picks", scripted.Step{Calls: []scripted.Call{{Name: "load_skill", Input: map[string]any{"name": "release-notes"}}}})
	if _, err := e.TestSkill(ctx, SkillTestRequest{Name: "release-notes", ProviderID: "picks", Model: "m2", Prompt: "Draft the notes"}); err != nil {
		t.Fatal(err)
	}
	rep, err := e.UsageReport(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	var test *UsageRow
	for i, r := range rep.Rows {
		if r.Model == "m2" {
			test = &rep.Rows[i]
		}
	}
	if test == nil || test.SessionID != "" || test.InputTokens == 0 || test.Calls != 1 || test.Deleted {
		t.Fatalf("the skill test's row = %+v in %+v", test, rep.Rows)
	}
	days, _ := e.Usage(ctx, 1)
	var models []string
	for _, m := range days[0].Models {
		models = append(models, m.Model)
	}
	slices.Sort(models)
	if !slices.Equal(models, []string{"m1", "m2"}) {
		t.Errorf("the day's models = %v", models)
	}
}

// PRV-008: a day's usage is priced by the price that held on that day, so a price
// that changes does not re-price what was already spent.
func TestUsagePricedByItsDay(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	today := time.Now()
	change := today.AddDate(0, 0, -1) // the price doubled yesterday
	for _, at := range []time.Time{today.AddDate(0, 0, -2), today} {
		if err := e.store.AddSessionlessUsage(ctx, at, event.UsageData{InputTokens: 1_000_000, ProviderID: "p", Model: "dear-1"}); err != nil {
			t.Fatal(err)
		}
	}
	c := e.Config()
	c.Prices = []config.Price{
		{Model: "dear-*", From: change.Format(time.DateOnly), Input: 8},
		{Model: "dear-*", Input: 4},
	}
	if _, err := e.SaveSettings(c); err != nil {
		t.Fatal(err)
	}
	days, err := e.Usage(ctx, 7)
	if err != nil || len(days) != 2 {
		t.Fatalf("days = %+v, %v", days, err)
	}
	if days[0].Cost != 8 || days[1].Cost != 4 {
		t.Errorf("costs, newest first = %v and %v, want 8 and 4", days[0].Cost, days[1].Cost)
	}
	rep, _ := e.UsageReport(ctx, 7)
	if len(rep.Rows) != 2 || rep.Rows[0].Cost != 8 || rep.Rows[1].Cost != 4 {
		t.Errorf("report rows = %+v", rep.Rows)
	}
}
