package permission

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// Linux's keyrings, browser profiles and password stores are closed like macOS's
// Keychain and browsers, on every system, and the macOS entries stay.
func TestKeyringsBrowsersAndPasswordStoresAreProtected(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	p := NewPolicy(home, filepath.Join(home, ".local", "share", "freeloop"))
	for _, rel := range []string{
		".local/share/keyrings/login.keyring", ".local/share/kwalletd/kdewallet.kwl",
		".mozilla/firefox/abc.default/cookies.sqlite", ".config/google-chrome/Default/Login Data",
		".config/chromium/Default/Cookies", ".config/BraveSoftware/Brave-Browser/Default/Cookies",
		".config/microsoft-edge/Default/Cookies", ".config/vivaldi/Default/Cookies", ".config/opera/Cookies",
		"snap/firefox/common/.mozilla/firefox/x/key4.db", ".var/app/org.mozilla.firefox/.mozilla/firefox/x/key4.db",
		".pki/nssdb/key4.db", ".password-store/github.com.gpg", ".config/Bitwarden/data.json",
		".config/KeePassXC/keepassxc.ini", ".local/share/freeloop/config.yaml",
		// What was there before.
		"Library/Keychains/login.keychain-db", "Library/Application Support/Google/Chrome/Default/Cookies", ".ssh/id_ed25519",
	} {
		for _, path := range []string{filepath.Join(home, rel), filepath.Dir(filepath.Join(home, rel))} {
			if p.readRefusal(path) == "" {
				t.Errorf("%s is readable", path)
			}
		}
		if p.writeRefusal(filepath.Join(home, "repo"), filepath.Join(home, rel)) == "" {
			t.Errorf("%s is writable", rel)
		}
	}
	// Neighbours stay open.
	for _, rel := range []string{".config/nvim/init.lua", ".local/share/nvim/x", ".config/google-chrome-notes", ".mozilla-notes/a"} {
		if why := p.readRefusal(filepath.Join(home, rel)); why != "" {
			t.Errorf("%s refused: %s", rel, why)
		}
	}
}

// Windows' credential stores, DPAPI's keys and the browsers under AppData are closed
// on every system, and so are the ones under a moved %APPDATA% (NFR-XPLAT-003).
func TestWindowsSecretsAreProtected(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	p := NewPolicy(home, filepath.Join(home, "AppData", "Roaming", "freeloop"))
	for _, rel := range []string{
		"AppData/Roaming/Microsoft/Credentials/ABC", "AppData/Local/Microsoft/Credentials/ABC",
		"AppData/Roaming/Microsoft/Protect/S-1-5-21/key", "AppData/Roaming/Microsoft/Vault/x",
		"AppData/Roaming/Microsoft/Crypto/RSA/key", "AppData/Roaming/Mozilla/Firefox/Profiles/x/key4.db",
		"AppData/Local/Google/Chrome/User Data/Default/Cookies", "AppData/Local/Microsoft/Edge/User Data/Default/Login Data",
		"AppData/Local/BraveSoftware/Brave-Browser/User Data/Default/Cookies", "AppData/Roaming/GitHub CLI/hosts.yml",
		"AppData/Roaming/gcloud/credentials.db", "AppData/Roaming/freeloop/config.yaml", "_netrc",
		".ssh/id_ed25519", ".aws/credentials", ".azure/msal_token_cache.json", ".docker/config.json", ".git-credentials", ".npmrc",
	} {
		path := filepath.Join(home, filepath.FromSlash(rel))
		if p.readRefusal(path) == "" {
			t.Errorf("%s is readable", rel)
		}
		if p.writeRefusal(filepath.Join(home, "repo"), path) == "" {
			t.Errorf("%s is writable", rel)
		}
	}
	for _, rel := range []string{"AppData/Local/Programs/Microsoft VS Code/Code.exe", "AppData/Roaming/npm/node_modules/x", "AppData/Local/Temp/x"} {
		if why := p.readRefusal(filepath.Join(home, filepath.FromSlash(rel))); why != "" {
			t.Errorf("%s refused: %s", rel, why)
		}
	}
	moved := filepath.Join(home, "elsewhere")
	got := windowsSecrets(home, filepath.Join(moved, "Roaming"), filepath.Join(moved, "Local"))
	for _, want := range []string{filepath.Join(moved, "Roaming", "Microsoft", "Protect"), filepath.Join(moved, "Local", "Google", "Chrome"), filepath.Join(home, "AppData", "Roaming", "Microsoft", "Protect")} {
		if !slices.Contains(got, want) {
			t.Errorf("%s is not protected", want)
		}
	}
	if n := len(windowsSecrets(home, "relative", "")); n != len(windowsSecrets(home, "", "")) {
		t.Error("a relative %APPDATA% was taken")
	}
}
