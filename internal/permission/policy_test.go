package permission

import (
	"path/filepath"
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
