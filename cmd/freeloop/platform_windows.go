package main

import "github.com/FinanceNik/Freeloop/internal/platform/windows"

// The Windows platform. Its keyring is the Credential Manager. It has no native
// sandbox yet (OQ-044): a sandboxed environment says so and runs nothing, and the
// Docker environment is the confined one. Nothing reads the process table: each
// command's job ends with the app (internal/environment/proc_windows.go), so there is
// no ledger of leftovers to check.
var thisPlatform = platform{
	Keys:       windows.CredentialManager{},
	EditorArgv: windows.EditorArgv,
	PDFArgv:    windows.PDFArgv,
	FolderArgv: windows.FolderArgv,
	SystemInfo: windows.SystemInfo,
	NoNap:      windows.NoNap,
	KeepAwake:  windows.KeepAwake,
}
