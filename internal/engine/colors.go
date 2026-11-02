package engine

import (
	"fmt"
	"path/filepath"

	"github.com/FinanceNik/Freeloop/internal/config"
)

// loadColors rereads the colors file (GUI-022). A file that cannot be read
// keeps the colors in force, so a half-saved edit does not flash the window.
func (e *Engine) loadColors() {
	c, issues := config.LoadColors(e.dataDir)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.issues[issueColors] = issues
	if !config.Unreadable(issues) || e.colors.Version == 0 {
		e.colors = c
	}
}

// Colors returns the colors chosen over the app's own.
func (e *Engine) Colors() config.Colors {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.colors
}

// SaveColors replaces the colors file.
func (e *Engine) SaveColors(c config.Colors) (config.Colors, error) {
	e.mu.Lock()
	broken := config.Unreadable(e.issues[issueColors])
	e.mu.Unlock()
	if broken {
		return config.Colors{}, fmt.Errorf("%s cannot be read, so it is not overwritten; fix or remove it first", config.ColorsFileName)
	}
	if err := config.SaveColors(e.dataDir, c); err != nil {
		return config.Colors{}, err
	}
	e.loadColors()
	return e.Colors(), nil
}

// ColorsFile returns the colors file's path for opening by hand, writing the
// current colors first when the file does not exist yet.
func (e *Engine) ColorsFile() (string, error) {
	if _, ok := config.Fingerprint(e.dataDir)[config.ColorsFileName]; !ok {
		if err := config.SaveColors(e.dataDir, e.Colors()); err != nil {
			return "", err
		}
	}
	return filepath.Join(e.dataDir, config.ColorsFileName), nil
}
