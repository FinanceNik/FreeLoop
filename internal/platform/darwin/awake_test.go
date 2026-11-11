//go:build darwin

package darwin

import (
	"os/exec"
	"strings"
	"testing"
)

// GUI-052: the assertion shows in pmset while held, and goes when released.
func TestKeepAwake(t *testing.T) {
	assertions := func() string {
		out, err := exec.Command("pmset", "-g", "assertions").Output()
		if err != nil {
			t.Skip("pmset unavailable")
		}
		return string(out)
	}
	release := KeepAwake("Freeloop test: keeping awake")
	if !strings.Contains(assertions(), "Freeloop test: keeping awake") {
		t.Error("no assertion while held")
	}
	release()
	release()
	if strings.Contains(assertions(), "Freeloop test: keeping awake") {
		t.Error("the assertion outlived its release")
	}
	NoNap("Freeloop test: no nap")()
}
