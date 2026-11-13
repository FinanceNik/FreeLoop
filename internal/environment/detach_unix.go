//go:build unix

package environment

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

// StartDetached starts a program that outlives the app: in a session of its
// own, with no terminal, not in the ledger of what the app stops when it
// ends. It is for one thing, starting the new copy of the app after an update
// in place (SET-008), and nothing an agent or a session asks for goes through
// it.
func StartDetached(argv []string) error {
	if len(argv) == 0 || argv[0] == "" {
		return errors.New("environment: no command to start")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("environment: start %s: %w", argv[0], err)
	}
	return cmd.Process.Release()
}
