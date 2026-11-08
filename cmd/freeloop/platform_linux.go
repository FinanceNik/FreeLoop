package main

import "github.com/FinanceNik/Freeloop/internal/platform/linux"

// The Linux platform. Its keyring is the Secret Service, GNOME Keyring or KWallet, and
// its sandbox bubblewrap (OQ-037).
var thisPlatform = platform{
	Keys:           linux.NewSecretService(),
	ProcessStarted: linux.ProcessStart,
	GroupMembers:   linux.GroupMembers,
	EditorArgv:     linux.EditorArgv,
	PDFArgv:        linux.PDFArgv,
	FolderArgv:     linux.FolderArgv,
	SystemInfo:     linux.SystemInfo,
	Confiner:       linux.Bubblewrap{},
}
