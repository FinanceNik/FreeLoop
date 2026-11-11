package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

// StylesDir holds the user's saved looks (GUI-053): each a name, the colors
// for both appearances and the two fonts, in one JSON file like colors.json,
// so a style can be handed around as one small file.
const StylesDir = "styles"

const maxStyleName = 64

// Style is one saved look. Its ID is its file's name; the values pass the
// same checks as colors.json and ui.textFont and ui.codeFont, so a style
// from someone else can neither add a rule to the page nor make it fetch
// something.
type Style struct {
	ID       string            `json:"id"`
	Version  int               `json:"version"`
	Name     string            `json:"name"`
	TextFont string            `json:"textFont"`
	CodeFont string            `json:"codeFont"`
	Light    map[string]string `json:"light"`
	Dark     map[string]string `json:"dark"`
}

// styleFile is what a style's file holds: the style without its ID.
type styleFile struct {
	Version  int               `json:"version"`
	Name     string            `json:"name"`
	TextFont string            `json:"textFont"`
	CodeFont string            `json:"codeFont"`
	Light    map[string]string `json:"light"`
	Dark     map[string]string `json:"dark"`
}

// StyleFile is a style's file relative to the data folder.
func StyleFile(id string) string { return filepath.Join(StylesDir, id+".json") }

// LoadStyles reads every styles/*.json in dir, sorted by name. Entries that
// fail the checks are left out and reported; a file that cannot be parsed is
// left out and reported as unreadable.
func LoadStyles(dir string) ([]Style, []Issue) {
	entries, err := os.ReadDir(filepath.Join(dir, StylesDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []Issue{{File: StylesDir, Message: err.Error()}}
	}
	var out []Style
	var issues []Issue
	for _, de := range entries {
		id, ok := strings.CutSuffix(de.Name(), ".json")
		if !ok || de.IsDir() || strings.HasPrefix(id, ".") {
			continue
		}
		rel := filepath.Join(StylesDir, de.Name())
		if !idPattern.MatchString(id) {
			issues = append(issues, Issue{File: rel, Message: "a style's file name is lower-case letters, digits and dashes; ignored"})
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			issues = append(issues, Issue{File: rel, Message: err.Error(), Unreadable: true})
			continue
		}
		var f styleFile
		if err := json.Unmarshal(b, &f); err != nil {
			issues = append(issues, Issue{File: rel, Message: err.Error(), Unreadable: true})
			continue
		}
		if f.Version > 1 {
			issues = append(issues, Issue{File: rel, Message: fmt.Sprintf("unsupported version %d; this build reads version 1", f.Version), Unreadable: true})
			continue
		}
		s, more := cleanStyle(id, f)
		for i := range more {
			more[i].File = rel
		}
		out = append(out, s)
		issues = append(issues, more...)
	}
	slices.SortFunc(out, func(a, b Style) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	slices.SortStableFunc(issues, func(a, b Issue) int {
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		return strings.Compare(a.Field, b.Field)
	})
	return out, issues
}

// cleanStyle drops what fails the checks and reports it, without a file.
func cleanStyle(id string, f styleFile) (Style, []Issue) {
	s := Style{ID: id, Version: 1, Name: strings.TrimSpace(f.Name), TextFont: f.TextFont, CodeFont: f.CodeFont}
	if s.Name == "" {
		s.Name = id
	}
	var issues []Issue
	if utf8.RuneCountInString(s.Name) > maxStyleName {
		issues = append(issues, Issue{Field: "name", Message: fmt.Sprintf("longer than %d characters; cut short", maxStyleName)})
		s.Name = string([]rune(s.Name)[:maxStyleName])
	}
	var light, dark []Issue
	s.Light, light = cleanColors("light", f.Light)
	s.Dark, dark = cleanColors("dark", f.Dark)
	issues = append(issues, light...)
	issues = append(issues, dark...)
	issues = append(issues, validateFont("textFont", &s.TextFont)...)
	issues = append(issues, validateFont("codeFont", &s.CodeFont)...)
	for i := range issues {
		issues[i].File = ""
	}
	return s, issues
}

// SaveStyle writes styles/<id>.json, refusing anything LoadStyles would drop
// or change.
func SaveStyle(dir string, s Style) error {
	if !idPattern.MatchString(s.ID) {
		return fmt.Errorf("bad style id %q", s.ID)
	}
	name := strings.TrimSpace(s.Name)
	switch {
	case name == "":
		return errors.New("a style needs a name")
	case utf8.RuneCountInString(name) > maxStyleName:
		return fmt.Errorf("a style's name is at most %d characters", maxStyleName)
	}
	f := styleFile{Version: 1, Name: name, TextFont: s.TextFont, CodeFont: s.CodeFont, Light: s.Light, Dark: s.Dark}
	clean, issues := cleanStyle(s.ID, f)
	if len(issues) > 0 {
		return fmt.Errorf("%s: %s", issues[0].Field, issues[0].Message)
	}
	b, err := json.MarshalIndent(styleFile{Version: 1, Name: clean.Name, TextFont: clean.TextFont, CodeFont: clean.CodeFont, Light: clean.Light, Dark: clean.Dark}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, StylesDir), 0o700); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, StyleFile(s.ID)), append(b, '\n'))
}

// StylesFolder returns the styles folder under dir, making it if missing.
func StylesFolder(dir string) (string, error) {
	folder := filepath.Join(dir, StylesDir)
	return folder, os.MkdirAll(folder, 0o700)
}

// DeleteStyle removes styles/<id>.json; a missing one is fine.
func DeleteStyle(dir, id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("bad style id %q", id)
	}
	err := os.Remove(filepath.Join(dir, StyleFile(id)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
