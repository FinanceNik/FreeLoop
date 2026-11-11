package config

import (
	"bytes"
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

	"github.com/FinanceNik/Freeloop/internal/permission"
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
	// A backslash or a drive's colon would be a separator or a drive on Windows.
	if strings.Contains(rel, "..") || strings.HasPrefix(rel, "/") || strings.ContainsAny(rel, `\:`) {
		return false
	}
	dir, name := path.Split(rel)
	dir = strings.TrimSuffix(dir, "/")
	if dir == StylesDir {
		return strings.HasSuffix(name, ".json") && !strings.HasPrefix(name, ".")
	}
	if !strings.HasSuffix(rel, ".yaml") {
		return false
	}
	if slices.Contains([]string{FileName, ProvidersFileName, RulesFileName, MCPFileName, CapabilitiesFileName}, rel) {
		return true
	}
	if name == "" || strings.HasPrefix(name, ".") {
		return false
	}
	return dir == EnvironmentsDir || dir == ProfilesDir || dir == PresetsDir || dir == OptimizersDir || dir == WorkflowsDir
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

// keepLocalOnly keeps local-only mode on in a restored config.yaml when it is on
// now (NFR-OFF-003): it blocks connections on this computer, and a file from elsewhere
// has no say in that. The user turns it off in Settings. Done before anything is
// written, so the watcher never sees the switch off.
func keepLocalOnly(dir string, content []byte) ([]byte, error) {
	current, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		return content, nil
	}
	if now, _ := parse(current); !now.Network.LocalOnly {
		return content, nil
	}
	next, issues := parse(content)
	// One that cannot be read leaves the app on the last that could, this one.
	if next.Network.LocalOnly || Unreadable(issues) {
		return content, nil
	}
	next.Network.LocalOnly = true
	return yaml.Marshal(next)
}

// Snapshot is what a folder's config files say, loaded as the app loads them.
type Snapshot struct {
	Config       Config
	Rules        []permission.Rule
	Environments []Environment
	Profiles     []Profile
	Presets      []HarnessPreset
	Providers    []Provider
	Optimizers   []Optimizer
	MCP          []MCPServer
}

// LoadSnapshot reads a folder's config files. What cannot be read is left out, as
// the app leaves it out, and the files themselves report it once restored.
func LoadSnapshot(dir string, tools []string) Snapshot {
	var s Snapshot
	s.Config, _, _ = Load(dir)
	s.Rules, _, _ = LoadRules(dir)
	s.Environments, _, _ = LoadEnvironments(dir)
	s.Profiles, _, _ = LoadProfiles(dir, tools)
	s.Presets, _ = LoadPresets(dir)
	s.Providers, _, _ = LoadProviders(dir)
	s.Optimizers, _ = LoadOptimizers(dir)
	s.MCP, _, _ = LoadMCP(dir)
	return s
}

// Kinds of a restore's change to a file.
const (
	FileNew      = "new"
	FileReplaced = "replaced"
	FileSame     = "same"
)

// BundleFile is what restoring a backup would do to one file.
type BundleFile struct {
	Path   string `json:"path"`
	Change string `json:"change"`
}

// PreviewBundle says what ImportBundle would leave in dir, without changing it:
// the restore runs on a copy of the folder's config files, in a temporary folder.
func PreviewBundle(dir string, data []byte, tools []string) (Snapshot, []BundleFile, error) {
	current, err := ExportBundle(dir)
	if err != nil {
		return Snapshot{}, nil, err
	}
	copied, err := os.MkdirTemp("", "freeloop-restore-*")
	if err != nil {
		return Snapshot{}, nil, fmt.Errorf("config: %w", err)
	}
	defer os.RemoveAll(copied)
	if _, err := ImportBundle(copied, current); err != nil {
		return Snapshot{}, nil, err
	}
	written, err := ImportBundle(copied, data)
	if err != nil {
		return Snapshot{}, nil, err
	}
	files := make([]BundleFile, 0, len(written))
	for _, rel := range written {
		f := BundleFile{Path: rel, Change: FileReplaced}
		old, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if errors.Is(err, os.ErrNotExist) {
			f.Change = FileNew
		} else if next, nextErr := os.ReadFile(filepath.Join(copied, filepath.FromSlash(rel))); err == nil && nextErr == nil && bytes.Equal(old, next) {
			f.Change = FileSame
		}
		files = append(files, f)
	}
	return LoadSnapshot(copied, tools), files, nil
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
		if rel == FileName {
			kept, err := keepLocalOnly(dir, []byte(content))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", rel, err)
			}
			b.Files[rel] = string(kept)
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
