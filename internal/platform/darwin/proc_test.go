//go:build darwin

package darwin

import (
	"os"
	"testing"
	"time"
)

func TestProcessStart(t *testing.T) {
	started, ok := ProcessStart(os.Getpid())
	if !ok || started.After(time.Now()) || time.Since(started) > time.Hour {
		t.Errorf("this process started %v, %v", started, ok)
	}
	if again, _ := ProcessStart(os.Getpid()); !again.Equal(started) {
		t.Errorf("the start time moved: %v then %v", started, again)
	}
	if _, ok := ProcessStart(999999); ok {
		t.Error("a process that does not exist has a start time")
	}
}
