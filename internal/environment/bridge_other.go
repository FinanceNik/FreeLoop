//go:build !linux

package environment

import (
	"fmt"
	"os"
)

// RunSandboxBridge is Linux's bridge to the egress proxy (bridge_linux.go). No
// other system's sandbox starts one.
func RunSandboxBridge([]string) int {
	fmt.Fprintln(os.Stderr, "freeloop: the sandbox's bridge runs on Linux only")
	return 2
}
