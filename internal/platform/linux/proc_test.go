//go:build linux

package linux

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
	if _, ok := ProcessStart(999999999); ok {
		t.Error("a process that does not exist has a start time")
	}
	if _, ok := ProcessStart(0); ok {
		t.Error("process 0 has a start time")
	}
}

// A later process starts later, to the clock tick.
func TestProcessStartOrdersProcesses(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	mine, _ := ProcessStart(os.Getpid())
	child, ok := ProcessStart(cmd.Process.Pid)
	if !ok || child.Before(mine) {
		t.Errorf("child %v, parent %v, %v", child, mine, ok)
	}
}

// The command's name can hold spaces and parentheses; the fields are counted after the last one.
func TestParseStatWithAnAwkwardName(t *testing.T) {
	line := "1234 (a b) c) S 1 1234 1234 0 -1 4194560 100 0 0 0 1 2 0 0 20 0 1 0 987654 1000 100 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0\n"
	f := parseStat(line)
	if f[0] != "S" || f[fieldPgrp] != "1234" || f[fieldStartTime] != "987654" {
		t.Errorf("fields %q", f)
	}
	if parseStat("garbage") != nil {
		t.Error("parsed garbage")
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
	if len(GroupMembers(999999999)) != 0 || GroupMembers(1) != nil {
		t.Error("a group that does not exist has members")
	}
}
