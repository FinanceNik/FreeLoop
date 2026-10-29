package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Fingerprint describes the hand-editable files under dir, by size and
// modification time, so a poll can tell which of them changed (SET-001).
// Keys are paths relative to dir; a missing file has no key.
func Fingerprint(dir string) map[string]string {
	out := map[string]string{}
	add := func(rel string) {
		if fi, err := os.Stat(filepath.Join(dir, rel)); err == nil && fi.Mode().IsRegular() {
			out[rel] = fmt.Sprintf("%d/%d", fi.Size(), fi.ModTime().UnixNano())
		}
	}
	add(FileName)
	add(ProvidersFileName)
	add(RulesFileName)
	add(MCPFileName)
	for _, sub := range []string{EnvironmentsDir, ProfilesDir, PresetsDir} {
		if entries, err := os.ReadDir(filepath.Join(dir, sub)); err == nil {
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".yaml") {
					add(filepath.Join(sub, e.Name()))
				}
			}
		}
	}
	return out
}

// Changed returns the keys whose entries differ between two fingerprints.
func Changed(before, after map[string]string) []string {
	var out []string
	for k, v := range after {
		if before[k] != v {
			out = append(out, k)
		}
	}
	for k := range before {
		if _, ok := after[k]; !ok {
			out = append(out, k)
		}
	}
	return out
}
