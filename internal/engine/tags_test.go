package engine

import (
	"context"
	"slices"
	"testing"
)

// Tags are cleaned, kept in the index, and survive a restart (SES-013).
func TestTags(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	ses, _ := demoSession(t, e, "demo", "ask")
	got, err := e.SetTags(ctx, ses.ID, []string{" #Bug ", "needs review", "bug", ""})
	if err != nil || !slices.Equal(got, []string{"bug", "needs-review"}) {
		t.Fatalf("tags = %v, %v", got, err)
	}
	if _, err := e.SetTags(ctx, ses.ID, make([]string, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SetTags(ctx, ses.ID, []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q"}); err == nil {
		t.Error("17 tags accepted")
	}
	e.SetTags(ctx, ses.ID, []string{"release"})
	e.Close()

	e = openTest(t, dir)
	defer e.Close()
	list, _ := e.store.ListSessions(ctx)
	if len(list) != 1 || !slices.Equal(list[0].Tags, []string{"release"}) {
		t.Errorf("after restart: %+v", list)
	}
	if n := NormalizeTags([]string{"Ünïcode Tag that is far longer than thirty-two characters"}); len([]rune(n[0])) != 32 {
		t.Errorf("long tag = %q", n[0])
	}
}
