package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// SessionSkillsDir holds, per session of an external agent, a plugin folder
// with the skills enabled for it (SKL-008): the app's own copy, so nothing
// is written where the agent keeps its own configuration.
const SessionSkillsDir = "session-skills"

var sessionID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// SessionPluginDir is where session sid's plugin folder is.
func SessionPluginDir(dir, sid string) string {
	return filepath.Join(dir, SessionSkillsDir, sid)
}

// WriteSessionPlugin replaces the contents of session sid's plugin folder
// with files, which map slash-separated paths inside it to their contents;
// files under a scripts/ folder are made executable. The folder itself
// stays, so a container that mounts it sees the new contents. It returns
// the folder.
func WriteSessionPlugin(dir, sid string, files map[string][]byte) (string, error) {
	if !sessionID.MatchString(sid) {
		return "", fmt.Errorf("session ID %q cannot name a folder", sid)
	}
	final := SessionPluginDir(dir, sid)
	if err := os.MkdirAll(final, 0o700); err != nil {
		return "", err
	}
	old, err := os.ReadDir(final)
	if err != nil {
		return "", err
	}
	for _, de := range old {
		if err := os.RemoveAll(filepath.Join(final, de.Name())); err != nil {
			return "", err
		}
	}
	for p, b := range files {
		clean := path.Clean(p)
		if clean == "." || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			return "", fmt.Errorf("%q is not a path inside the plugin", p)
		}
		full := filepath.Join(final, filepath.FromSlash(clean))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			return "", err
		}
		mode := os.FileMode(0o600)
		if strings.Contains("/"+clean, "/scripts/") {
			mode = 0o700
		}
		if err := os.WriteFile(full, b, mode); err != nil {
			return "", err
		}
	}
	return final, nil
}

// RemoveSessionPlugin deletes session sid's plugin folder, if any.
func RemoveSessionPlugin(dir, sid string) error {
	if !sessionID.MatchString(sid) {
		return nil
	}
	return os.RemoveAll(SessionPluginDir(dir, sid))
}
