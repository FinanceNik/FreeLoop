package windows

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// uninstallKey is where the installer records the app (build/windows/installer.nsi).
const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Freeloop`

// recordVersion tells Windows' list of installed apps the new version, for a copy
// the installer put in dir. A portable copy has no entry there and none is made.
func recordVersion(dir, version string) {
	if _, err := os.Stat(filepath.Join(dir, "uninstall.exe")); err != nil {
		return
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.SET_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return
	}
	defer k.Close()
	k.SetStringValue("DisplayVersion", version)
}
