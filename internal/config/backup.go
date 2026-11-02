package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config backup (SET-005): the hand-editable files of the data folder in
// one JSON file, without secrets: those live in the Keychain, and the files
// only refer to them. Skills, sessions and the database are not included.

// BundleFormat marks a backup file.
const BundleFormat = "freeloop-config"

// Bundle is a backup: file contents by path relative to the data folder.
type Bundle struct {
	Format   string            `json:"format"`
	Version  int               `json:"version"`
	Exported time.Time         `json:"exported"`
	Files    map[string]string `json:"files"`
}

// maxBundleFile caps one file in a bundle.
const maxBundleFile = 1 << 20

// backedUp reports whether rel (with /) is a file a bundle may hold.
func backedUp(rel string) bool {
	if rel == ColorsFileName {
		return true
	}
	if strings.Contains(rel, "..") || strings.HasPrefix(rel, "/") || !strings.HasSuffix(rel, ".yaml") {
		return false
	}
	if slices.Contains([]string{FileName, ProvidersFileName, RulesFileName, MCPFileName, CapabilitiesFileName}, rel) {
		return true
	}
	dir, name := path.Split(rel)
	if name == "" || strings.HasPrefix(name, ".") {
		return false
	}
	dir = strings.TrimSuffix(dir, "/")
	return dir == EnvironmentsDir || dir == ProfilesDir || dir == PresetsDir
}

// ExportBundle collects the data folder's config files.
func ExportBundle(dir string) ([]byte, error) {
	b := Bundle{Format: BundleFormat, Version: 1, Exported: time.Now().UTC(), Files: map[string]string{}}
	files := Fingerprint(dir)
	if _, err := os.Stat(filepath.Join(dir, CapabilitiesFileName)); err == nil {
		files[CapabilitiesFileName] = ""
	}
	for rel := range files {
		rel = filepath.ToSlash(rel)
		if !backedUp(rel) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return nil, fmt.Errorf("config: %w", err)
		}
		b.Files[rel] = string(data)
	}
	return json.MarshalIndent(b, "", "  ")
}

// withoutApprovals clears what an mcp.yaml says the user confirmed. A
// mark is only good on the machine that made it, but a restored backup asks
// again in any case (MCP-005).
func withoutApprovals(content []byte) ([]byte, error) {
	// Through the type the app reads, so a key spelled in some other way, as a
	// !!binary one is, decodes to the same field and cannot keep it.
	var f mcpFile
	if err := yaml.Unmarshal(content, &f); err != nil {
		return nil, err
	}
	for i := range f.Servers {
		f.Servers[i].Approved = ""
	}
	return yaml.Marshal(f)
}

// ImportBundle writes a backup's files into the data folder, after copying
// the files it replaces to backups/<time>/. Every file is checked first, so
// a bad backup changes nothing. It returns the paths written.
func ImportBundle(dir string, data []byte) ([]string, error) {
	var b Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("this is not a Freeloop config backup: %w", err)
	}
	if b.Format != BundleFormat || b.Version != 1 {
		return nil, errors.New("this is not a Freeloop config backup this version can read")
	}
	var names []string
	for rel, content := range b.Files {
		if !backedUp(rel) {
			return nil, fmt.Errorf("the backup holds %q, which is not a config file", rel)
		}
		if len(content) > maxBundleFile {
			return nil, fmt.Errorf("%s is larger than 1 MB", rel)
		}
		var v any
		if err := yaml.Unmarshal([]byte(content), &v); err != nil {
			return nil, fmt.Errorf("%s is not valid YAML: %w", rel, err)
		}
		if rel == MCPFileName {
			clean, err := withoutApprovals([]byte(content))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", rel, err)
			}
			b.Files[rel] = string(clean)
		}
		names = append(names, rel)
	}
	slices.Sort(names)
	saved := filepath.Join(dir, "backups", time.Now().Format("20060102-150405"))
	for _, rel := range names {
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if old, err := os.ReadFile(target); err == nil {
			keep := filepath.Join(saved, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(keep), 0o700); err != nil {
				return nil, fmt.Errorf("config: %w", err)
			}
			if err := os.WriteFile(keep, old, 0o600); err != nil {
				return nil, fmt.Errorf("config: %w", err)
			}
		}
	}
	for _, rel := range names {
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return nil, fmt.Errorf("config: %w", err)
		}
		if err := writeAtomic(target, []byte(b.Files[rel])); err != nil {
			return nil, err
		}
	}
	return names, nil
}
