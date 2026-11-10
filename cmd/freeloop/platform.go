package main

import (
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// platform is what the system under the app provides: the one place main.go depends
// on which system it is built for (NFR-XPLAT-002). thisPlatform is set in
// platform_mac.go, platform_linux.go and platform_windows.go; each imports its own package under
// internal/platform, and nothing else does.
type platform struct {
	// Keys is the system's keyring, the only place secrets are kept (SEC-004).
	Keys secrets.Store
	// Confiner enforces the native sandbox (ENV-009); nil where there is none yet, and a
	// sandboxed environment then runs nothing and says so.
	Confiner environment.Confiner
	// ProcessStarted and GroupMembers read the process table, for the ledger of
	// process groups (NFR-REL-005).
	ProcessStarted func(pid int) (time.Time, bool)
	GroupMembers   func(pgid int) []int
	// EditorArgv, PDFArgv and FolderArgv are the commands that show a file or a folder,
	// never running it.
	EditorArgv, PDFArgv, FolderArgv func(path string) []string
	// SystemInfo describes the machine for a problem report.
	SystemInfo func() string
	// MenuWords are the system's own names for menu items in a language, where they
	// differ from menuWords (GUI-048).
	MenuWords map[string]map[string]string
}
