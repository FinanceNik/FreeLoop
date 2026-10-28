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
