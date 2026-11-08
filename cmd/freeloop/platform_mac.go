// The macOS platform. There is no darwin constraint in this file's name or its
// build line: macOS-only code lives in internal/platform/darwin (NFR-XPLAT-002), and
// this file only picks it, for every system that is neither Linux nor Windows.

//go:build !linux && !windows

package main

import "github.com/FinanceNik/Freeloop/internal/platform/darwin"

var thisPlatform = platform{
	Keys:           darwin.Keychain{},
	Confiner:       darwin.Seatbelt{},
	ProcessStarted: darwin.ProcessStart,
	GroupMembers:   darwin.GroupMembers,
	EditorArgv:     darwin.EditorArgv,
	PDFArgv:        darwin.PDFArgv,
	FolderArgv:     darwin.FolderArgv,
	SystemInfo:     darwin.SystemInfo,
}
