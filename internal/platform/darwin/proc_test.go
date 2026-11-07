//go:build darwin

package darwin

import (
	"os"
	"os/exec"
	"slices"
	"syscall"
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

func TestGroupMembers(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30 & sleep 30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	var got []int
	for range 50 {
		if got = GroupMembers(cmd.Process.Pid); len(got) >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(got) < 2 || !slices.Contains(got, cmd.Process.Pid) {
		t.Errorf("the group's members: %v", got)
	}
	if GroupMembers(999999) != nil && len(GroupMembers(999999)) != 0 {
		t.Error("a group that does not exist has members")
	}
}
