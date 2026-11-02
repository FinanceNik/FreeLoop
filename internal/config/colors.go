package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
)

// ColorsFileName holds the colors chosen over the app's own (GUI-022). It is
// JSON rather than YAML so a theme can be copied around as one small file.
const ColorsFileName = "colors.json"

// Colors overrides the look's color tokens, by name without the leading
// dashes, for each appearance. A token not named keeps the app's color.
type Colors struct {
	Version int               `json:"version"`
	Light   map[string]string `json:"light"`
	Dark    map[string]string `json:"dark"`
}

// ColorTokens are the names a colors file may set: the tokens at the top of
// frontend/src/app.css that hold plain colors.
var ColorTokens = []string{
	"bg-app", "bg", "surface", "surface-2", "input",
	"border", "border-strong", "input-border",
	"text", "muted", "faint",
	"accent", "accent-hover", "accent-soft", "accent-text", "link", "focus",
	"ok", "warn", "danger", "danger-soft", "add", "del",
	"series-1", "series-2", "series-3", "series-4", "series-5", "series-other",
}

// The values go into a style sheet, so only plain colors pass: a value such
// as url(...) would make the page fetch something, and the app makes no
// connections the user did not configure.
var colorValue = regexp.MustCompile(`^(#[0-9a-fA-F]{3,4}|#[0-9a-fA-F]{6}|#[0-9a-fA-F]{8}|rgba?\([0-9., %/]+\))$`)

// ValidColor reports whether v is a hex or rgb() color.
func ValidColor(v string) bool { return colorValue.MatchString(v) }

func cleanColors(mode string, in map[string]string) (map[string]string, []Issue) {
	out := map[string]string{}
	var issues []Issue
	for k, v := range in {
		switch {
		case !slices.Contains(ColorTokens, k):
			issues = append(issues, Issue{File: ColorsFileName, Field: mode + "." + k, Message: "not a color this app has; ignored"})
		case !ValidColor(v):
			issues = append(issues, Issue{File: ColorsFileName, Field: mode + "." + k, Message: fmt.Sprintf("%q is not a hex or rgb() color; ignored", v)})
		default:
			out[k] = v
		}
	}
	return out, issues
}

func emptyColors() Colors {
	return Colors{Version: 1, Light: map[string]string{}, Dark: map[string]string{}}
}

// LoadColors reads the colors file. A missing file means no overrides;
// entries that fail the checks are left out and reported, and a file that
// cannot be parsed is reported as unreadable.
func LoadColors(dir string) (Colors, []Issue) {
	b, err := os.ReadFile(filepath.Join(dir, ColorsFileName))
	if errors.Is(err, os.ErrNotExist) {
		return emptyColors(), nil
	}
	if err != nil {
		return emptyColors(), []Issue{{File: ColorsFileName, Message: err.Error(), Unreadable: true}}
	}
	var raw Colors
	if err := json.Unmarshal(b, &raw); err != nil {
		return emptyColors(), []Issue{{File: ColorsFileName, Message: err.Error(), Unreadable: true}}
	}
	c := emptyColors()
	light, issues := cleanColors("light", raw.Light)
	dark, more := cleanColors("dark", raw.Dark)
	c.Light, c.Dark = light, dark
	issues = append(issues, more...)
	slices.SortFunc(issues, func(a, b Issue) int {
		if a.Field < b.Field {
			return -1
		}
		return 1
	})
	return c, issues
}

// SaveColors writes the colors file, refusing anything LoadColors would drop.
func SaveColors(dir string, c Colors) error {
	light, issues := cleanColors("light", c.Light)
	dark, more := cleanColors("dark", c.Dark)
	if issues = append(issues, more...); len(issues) > 0 {
		return fmt.Errorf("%s: %s", issues[0].Field, issues[0].Message)
	}
	b, err := json.MarshalIndent(Colors{Version: 1, Light: light, Dark: dark}, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, ColorsFileName), append(b, '\n'))
}
