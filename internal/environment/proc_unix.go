//go:build unix

package environment

import (
	"os/exec"
	"syscall"
)

// setProcessGroup starts the command in its own process group so that
// killProcessGroup ends everything it spawned.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// joinGroup finishes putting a command started with setProcessGroup in its group.
// On Unix the group exists from the start; Windows puts the process in a job here.
func joinGroup(*exec.Cmd) error { return nil }

// releaseGroup lets go of what holds an ended group; on Unix nothing does.
func releaseGroup(int) {}
