package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// SkillsDir is the app's skill store in the data folder (SKL-002).
const SkillsDir = "skills"

// ProvenanceFile records where a skill in the store came from (SKL-003).
// Its leading dot keeps it out of the skill's file list.
const ProvenanceFile = ".freeloop-source.yaml"

// SkillProvenance is the content of ProvenanceFile.
type SkillProvenance struct {
	Source     string    `yaml:"source" json:"source"`
	ImportedAt time.Time `yaml:"importedAt" json:"importedAt"`
	// Digest is a hash of the skill's files as installed, to tell an
	// unchanged source from a changed one on the next import.
	Digest string `yaml:"digest" json:"digest"`
	// Ref and Commit are set for a skill installed from a git repository
	// (Source is its URL): the ref asked for and the commit it was pinned
	// to (SKL-006).
	Ref    string `yaml:"ref,omitempty" json:"ref"`
	Commit string `yaml:"commit,omitempty" json:"commit"`
}

// previousDir keeps the version an install replaced, for a rollback.
const previousDir = ".previous"

var skillName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ReadSkillProvenance returns how a store skill was installed; false when
// it has no record.
func ReadSkillProvenance(dir, name string) (SkillProvenance, bool) {
	var p SkillProvenance
	if !skillName.MatchString(name) {
		return p, false
	}
	b, err := os.ReadFile(filepath.Join(dir, SkillsDir, name, ProvenanceFile))
	if err != nil || yaml.Unmarshal(b, &p) != nil {
		return p, false
	}
	return p, true
}

// InstallSkill writes a skill into the store as skills/<name>/, replacing
// any skill of that name in one rename, so a reader never sees half of it.
// files maps slash-separated paths inside the skill to their contents;
// files under scripts/ are made executable.
func InstallSkill(dir, name string, files map[string][]byte, prov SkillProvenance) error {
	if !skillName.MatchString(name) || len(name) > 64 {
		return fmt.Errorf("bad skill name %q", name)
	}
	if _, ok := files["SKILL.md"]; !ok {
		return errors.New("a skill needs a SKILL.md")
	}
	store := filepath.Join(dir, SkillsDir)
	if err := os.MkdirAll(store, 0o700); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	tmp, err := os.MkdirTemp(store, ".install-"+name+"-")
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	defer os.RemoveAll(tmp)
	for rel, b := range files {
		if !filepath.IsLocal(filepath.FromSlash(rel)) || strings.HasPrefix(filepath.Base(rel), ProvenanceFile) {
			return fmt.Errorf("bad file name %q in the skill", rel)
		}
		path := filepath.Join(tmp, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("config: %w", err)
		}
		mode := os.FileMode(0o600)
		if strings.HasPrefix(rel, "scripts/") {
			mode = 0o700
		}
		if err := os.WriteFile(path, b, mode); err != nil {
			return fmt.Errorf("config: %w", err)
		}
	}
	pb, err := yaml.Marshal(prov)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, ProvenanceFile), pb, 0o600); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	final := filepath.Join(store, name)
	old := ""
	if _, err := os.Stat(final); err == nil {
		old = filepath.Join(store, fmt.Sprintf(".old-%s-%d", name, time.Now().UnixNano()))
		if err := os.Rename(final, old); err != nil {
			return fmt.Errorf("config: %w", err)
		}
	}
	if err := os.Rename(tmp, final); err != nil {
		if old != "" {
			os.Rename(old, final)
		}
		return fmt.Errorf("config: %w", err)
	}
	if old != "" {
		// The replaced version stays for a rollback (SKL-006).
		prev := filepath.Join(store, previousDir, name)
		os.RemoveAll(prev)
		if err := os.MkdirAll(filepath.Dir(prev), 0o700); err != nil || os.Rename(old, prev) != nil {
			os.RemoveAll(old)
		}
	}
	return nil
}

// HasPreviousSkill reports whether an install kept a version to roll back to.
func HasPreviousSkill(dir, name string) bool {
	if !skillName.MatchString(name) {
		return false
	}
	fi, err := os.Stat(filepath.Join(dir, SkillsDir, previousDir, name, "SKILL.md"))
	return err == nil && fi.Mode().IsRegular()
}

// RollbackSkill swaps a skill with the version its last install replaced;
// doing it again undoes it.
func RollbackSkill(dir, name string) error {
	if !HasPreviousSkill(dir, name) {
		return fmt.Errorf("there is no earlier version of %s", name)
	}
	store := filepath.Join(dir, SkillsDir)
	final, prev := filepath.Join(store, name), filepath.Join(store, previousDir, name)
	swap := filepath.Join(store, fmt.Sprintf(".swap-%s-%d", name, time.Now().UnixNano()))
	if err := os.Rename(final, swap); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := os.Rename(prev, final); err != nil {
		os.Rename(swap, final)
		return fmt.Errorf("config: %w", err)
	}
	if err := os.Rename(swap, prev); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return nil
}

// cloneDir holds temporary checkouts of skill repositories.
const cloneDir = ".clones"

// NewSkillClone returns a fresh path, not yet created, for a checkout of a
// skill repository; RemoveSkillClone deletes it.
func NewSkillClone(dir string) (string, error) {
	base := filepath.Join(dir, SkillsDir, cloneDir)
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", fmt.Errorf("config: %w", err)
	}
	return filepath.Join(base, fmt.Sprintf("c%d", time.Now().UnixNano())), nil
}

// RemoveSkillClone deletes a checkout NewSkillClone named.
func RemoveSkillClone(dir, path string) error {
	base := filepath.Join(dir, SkillsDir, cloneDir) + string(filepath.Separator)
	if !strings.HasPrefix(filepath.Clean(path), base) {
		return fmt.Errorf("config: %s is not a skill checkout", path)
	}
	return os.RemoveAll(path)
}

// RemoveSkill deletes skills/<name>/ from the store.
func RemoveSkill(dir, name string) error {
	if !skillName.MatchString(name) {
		return fmt.Errorf("bad skill name %q", name)
	}
	return os.RemoveAll(filepath.Join(dir, SkillsDir, name))
}
