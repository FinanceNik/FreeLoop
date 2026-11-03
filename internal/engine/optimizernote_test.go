package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// HRN-019: the model hears about an optimizer once, when it comes on, and not at all
// without one, and the transcript does not show it as something the user said.
func TestOptimizerNote(t *testing.T) {
	ctx := context.Background()
	e := openBinding(t, t.TempDir(), secrets.NewMemory())
	withScript(e, "s", scripted.Step{Text: "one"}, scripted.Step{Text: "two"}, scripted.Step{Text: "three"})
	prov := e.providers.built["s"].p.(*scripted.Provider)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "s", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	ch, cancel := e.Subscribe()
	defer cancel()
	turn := func(text string) string {
		t.Helper()
		e.Prompt(ctx, ses.ID, text)
		waitIdle(t, ch, ses.ID)
		reqs := prov.Requests()
		msgs := reqs[len(reqs)-1].Messages
		var b strings.Builder
		for _, part := range msgs[len(msgs)-1].Parts {
			b.WriteString(part.Text)
		}
		return b.String()
	}
	if got := turn("first"); strings.Contains(got, "Note from Freeloop") {
		t.Errorf("no optimizer, and yet: %q", got)
	}
	e.emit(ses.ID, event.OptimizerChanged, event.OptimizerChangedData{Optimizer: "rtk", Name: "RTK"})
	got := turn("second")
	if !strings.Contains(got, "RTK is on") || !strings.Contains(got, "second") {
		t.Errorf("after choosing it: %q", got)
	}
	if got := turn("third"); strings.Contains(got, "Note from Freeloop") {
		t.Errorf("told twice: %q", got)
	}
	evs, _ := e.Events(ctx, ses.ID, 0)
	for _, ev := range evs {
		if ev.Type == event.UserMessageChunk && strings.Contains(string(ev.Data), "Note from Freeloop") {
			t.Errorf("the transcript shows it as the user's: %s", ev.Data)
		}
	}
}

func TestParseGain(t *testing.T) {
	got, err := parseGain([]byte("[warn] No hook installed\n{\"summary\":{\"total_commands\":58,\"total_input\":113559,\"total_output\":113338,\"total_saved\":221}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Commands != 58 || got.Input != 113559 || got.Output != 113338 || got.Saved != 221 {
		t.Errorf("%+v", got)
	}
	for _, bad := range []string{"", "No tracking data yet.", "{}", "{broken"} {
		if _, err := parseGain([]byte(bad)); err == nil {
			t.Errorf("%q read as a report", bad)
		}
	}
}
