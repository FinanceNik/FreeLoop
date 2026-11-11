package engine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
)

// loadStyles rereads the saved styles (GUI-053).
func (e *Engine) loadStyles() {
	styles, issues := config.LoadStyles(e.dataDir)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.styles = styles
	e.issues[issueStyles] = issues
}

// Styles returns the saved styles, sorted by name.
func (e *Engine) Styles() []config.Style {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.styles)
}

// style returns the saved style id, or a zero Style.
func (e *Engine) style(id string) config.Style {
	e.mu.Lock()
	defer e.mu.Unlock()
	if i := slices.IndexFunc(e.styles, func(s config.Style) bool { return s.ID == id }); i >= 0 {
		return e.styles[i]
	}
	return config.Style{}
}

// SaveStyle writes a style. One without an ID is new and gets an ID made from
// its name that no file in the folder has.
func (e *Engine) SaveStyle(s config.Style) (config.Style, error) {
	files := config.Fingerprint(e.dataDir)
	if s.ID == "" {
		base := strings.Trim(slugChars.ReplaceAllString(strings.ToLower(s.Name), "-"), "-")
		if len(base) > 56 {
			base = strings.TrimRight(base[:56], "-")
		}
		if base == "" {
			base = "style"
		}
		s.ID = base
		for n := 2; ; n++ {
			if _, taken := files[config.StyleFile(s.ID)]; !taken {
				break
			}
			s.ID = fmt.Sprintf("%s-%d", base, n)
		}
	} else {
		e.mu.Lock()
		broken := slices.ContainsFunc(e.issues[issueStyles], func(is config.Issue) bool {
			return is.Unreadable && is.File == config.StyleFile(s.ID)
		})
		e.mu.Unlock()
		if broken {
			return config.Style{}, fmt.Errorf("%s cannot be read, so it is not overwritten; fix or remove it first", config.StyleFile(s.ID))
		}
	}
	if err := config.SaveStyle(e.dataDir, s); err != nil {
		return config.Style{}, err
	}
	e.loadStyles()
	return e.style(s.ID), nil
}

// DeleteStyle removes a saved style. The look on screen stays as it is.
func (e *Engine) DeleteStyle(id string) error {
	if err := config.DeleteStyle(e.dataDir, id); err != nil {
		return err
	}
	e.loadStyles()
	return nil
}

// StylesFolder returns the styles folder, made if missing, for showing in the
// file manager: a style is shared by copying its file in or out.
func (e *Engine) StylesFolder() (string, error) {
	return config.StylesFolder(e.dataDir)
}
