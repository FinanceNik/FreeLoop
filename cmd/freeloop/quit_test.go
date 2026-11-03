package main

import (
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/engine"
)

func TestQuitMessage(t *testing.T) {
	for _, c := range []struct {
		in   engine.QuitCheck
		want string
	}{
		{engine.QuitCheck{Working: 1}, "1 session still working."},
		{engine.QuitCheck{Working: 3}, "3 sessions still working."},
		{engine.QuitCheck{Waiting: 2}, "2 approvals unanswered."},
		{engine.QuitCheck{Working: 1, Waiting: 1}, "1 session still working and 1 approval unanswered."},
	} {
		if got := quitMessage(c.in); !strings.HasPrefix(got, c.want) {
			t.Errorf("%+v: %q", c.in, got)
		}
	}
}
