package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// runTour starts the demo, answers its two questions (notes.md: allow; .env: allow)
// and returns the session's decisions by the path or command of the call they were for.
func runTour(t *testing.T, e *Engine) (sid string, decided map[string]permission.Decision, asked []string) {
	t.Helper()
	ctx := context.Background()
	ch, cancel := e.Subscribe()
	defer cancel()

	ses, err := e.StartDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ses.Tags, demoTag) || ses.ProviderID != TourProviderID || ses.Mode != "ask" {
		t.Fatalf("the demo session = %+v", ses)
	}
	for {
		ev := waitFor(t, ch, func(ev event.Event) bool {
			return ev.SID == ses.ID && (ev.Type == event.PermissionRequested || ev.Type == event.TurnEnded)
		})
		if ev.Type == event.TurnEnded {
			if !strings.Contains(string(ev.Data), "end_turn") {
				t.Fatalf("the turn ended with %s", ev.Data)
			}
			break
		}
		var req permission.Request
		if err := json.Unmarshal(ev.Data, &req); err != nil {
			t.Fatal(err)
		}
		asked = append(asked, filepath.Base(strings.Join(req.Paths, "")))
		if err := e.Decide(DecideRequest{RequestID: req.ID, OptionID: "allow_once"}); err != nil {
			t.Fatal(err)
		}
	}

	evs, err := e.Events(ctx, ses.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Each call by what it was about, then its decision.
	about := map[string]string{}
	decided = map[string]permission.Decision{}
	for _, ev := range evs {
		switch ev.Type {
		case event.ToolCall:
			var c struct {
				ToolCallID string          `json:"toolCallId"`
				RawInput   json.RawMessage `json:"rawInput"`
			}
			json.Unmarshal(ev.Data, &c)
			var in struct{ Path, Command string }
			json.Unmarshal(c.RawInput, &in)
			if c.ToolCallID != "" && (in.Path != "" || in.Command != "") {
				about[c.ToolCallID] = in.Path + in.Command
			}
		case event.PermissionDecided:
			var d permission.Decision
			json.Unmarshal(ev.Data, &d)
			decided[about[d.ToolCallID]] = d
		}
	}
	return ses.ID, decided, asked
}

func checkTour(t *testing.T, e *Engine, dataDir string) {
	t.Helper()
	sid, decided, asked := runTour(t, e)

	if len(asked) != 2 {
		t.Errorf("asked about %v, want notes.md and .env", asked)
	}
	if d := decided["README.md"]; d.Outcome != permission.Allowed || d.Actor != permission.ActorPolicy {
		t.Errorf("README.md = %+v, want allowed by policy", d)
	}
	if d := decided["notes.md"]; d.Outcome != permission.Allowed || d.Actor != permission.ActorUser {
		t.Errorf("notes.md = %+v, want allowed by the user", d)
	}
	if d := decided["git push origin main"]; d.Outcome != permission.Denied || d.Actor != permission.ActorRule || d.Rule == nil || d.Rule.Command != "git push *" {
		t.Errorf("git push = %+v, want denied by the session rule", d)
	}
	if d := decided[".git/hooks/pre-commit"]; d.Outcome != permission.Denied || d.Actor != permission.ActorProtectedPath {
		t.Errorf("the hook = %+v, want denied as a protected path", d)
	}
	if d := decided[".env"]; d.Outcome != permission.Allowed || d.Actor != permission.ActorUser {
		t.Errorf(".env = %+v, want asked, then allowed by the user", d)
	}

	root := dataDir + "-demo"
	if b, err := os.ReadFile(filepath.Join(root, "notes.md")); err != nil || !strings.Contains(string(b), "demo") {
		t.Errorf("notes.md: %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "hooks", "pre-commit")); err == nil {
		t.Error("the protected hook was written")
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "HEAD")); err != nil {
		t.Errorf("the demo project is not a git repository: %v", err)
	}

	// A second start returns the same session and starts nothing.
	again, err := e.StartDemo(context.Background())
	if err != nil || again.ID != sid {
		t.Errorf("a second StartDemo = %s, %v; want %s", again.ID, err, sid)
	}
	if got := len(e.PendingPermissions()); got != 0 {
		t.Errorf("%d requests pending after a second start", got)
	}

	// Only the demo session can be removed this way: the user's own tag "demo"
	// does not make a session the demo.
	if e.dev {
		other, _ := demoSession(t, e, "other", "ask")
		if _, err := e.SetTags(context.Background(), other.ID, []string{demoTag}); err != nil {
			t.Fatal(err)
		}
		if err := e.RemoveDemo(context.Background(), other.ID); err == nil {
			t.Error("RemoveDemo removed a session that is not the demo")
		}
		if _, err := e.store.GetSession(context.Background(), other.ID); err != nil {
			t.Errorf("the other session is gone: %v", err)
		}
		if again, err := e.StartDemo(context.Background()); err != nil || again.ID != sid {
			t.Errorf("StartDemo with a session tagged demo = %s, %v; want %s", again.ID, err, sid)
		}
	}
	demo, err := e.store.GetSession(context.Background(), sid)
	if err != nil || demo.ProjectID == "" {
		t.Fatalf("demo session has no project: %+v, %v", demo, err)
	}
	if err := e.RemoveDemo(context.Background(), sid); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.GetSession(context.Background(), sid); err == nil {
		t.Error("the demo session is still there")
	}
	if _, err := os.Stat(dataDir + "-demo"); !os.IsNotExist(err) {
		t.Errorf("the demo folder is still there: %v", err)
	}
	if _, err := e.store.GetProject(context.Background(), demo.ProjectID); err == nil {
		t.Error("the demo project is still listed")
	}
	// And it can be started again.
	fresh, err := e.StartDemo(context.Background())
	if err != nil || fresh.ID == sid {
		t.Errorf("StartDemo after a removal = %s, %v", fresh.ID, err)
	}
}

func TestDemoTourMeetsEachAnswerOfThePolicy(t *testing.T) {
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	checkTour(t, e, dir)
}

func TestDemoTourWorksInReleaseBuildsAndStaysOutOfPickers(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	for _, p := range e.ListProviders() {
		if p.ID == TourProviderID || p.ID == DemoProviderID {
			t.Errorf("provider %s is listed", p.ID)
		}
	}
	checkTour(t, e, dir)
	for _, p := range e.ListProviders() {
		if p.ID == TourProviderID {
			t.Errorf("provider %s is listed after the tour", p.ID)
		}
	}
}

// The demo is the built-in agent on the tour whatever the user's defaults say: a
// default profile on an external agent once ran the demo's prompt on that agent.
func TestDemoIgnoresTheDefaultProfile(t *testing.T) {
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	if _, err := e.SaveProfile(config.Profile{ID: config.DefaultProfileID, Name: "Default", Harness: "opencode"}); err != nil {
		t.Fatal(err)
	}
	if p, _ := e.profile(""); p.Harness != "opencode" {
		t.Fatalf("default profile = %+v", p)
	}
	ses, err := e.StartDemo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ses.HarnessID != builtin.ID || ses.ProviderID != TourProviderID || ses.ProfileID != TourProfileID || (ses.Optimizer != "" && ses.Optimizer != config.OptimizerOff) {
		t.Errorf("demo session = harness %q, provider %q, profile %q, optimizer %q", ses.HarnessID, ses.ProviderID, ses.ProfileID, ses.Optimizer)
	}
	if err := e.RemoveDemo(context.Background(), ses.ID); err != nil {
		t.Fatal(err)
	}
	checkTour(t, e, dir)
	if _, err := e.SaveProfile(tourProfile()); err == nil {
		t.Error("the demo's profile could be saved over")
	}
	for _, p := range e.ListProfiles() {
		if p.ID == TourProfileID {
			t.Error("the demo's profile is listed")
		}
	}
}

func TestRemoveOwnedRefusesWhatIsOutsideItsRoot(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "demo")
	os.MkdirAll(inside, 0o755)
	outside := t.TempDir()
	if err := environment.RemoveOwned(root, outside); err == nil {
		t.Error("removed a folder outside the root")
	}
	if err := environment.RemoveOwned(root, root); err == nil {
		t.Error("removed the root")
	}
	if err := environment.RemoveOwned(root, filepath.Join(inside, "..", "..")); err == nil {
		t.Error("removed the root's parent")
	}
	if err := environment.RemoveOwned(root, inside); err != nil || environment.IsDir(inside) {
		t.Errorf("did not remove the folder inside the root: %v", err)
	}
	if err := environment.RemoveOwned(root, inside); err != nil {
		t.Errorf("a folder already gone is an error: %v", err)
	}
}
