package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// UpdateStateFile keeps the last update check's result, so the app looks
// at most once a day (OQ-015).
const UpdateStateFile = "update-check.json"

// UpdateState is the last update check.
type UpdateState struct {
	CheckedAt time.Time `json:"checkedAt"`
	Latest    string    `json:"latest"`
	URL       string    `json:"url"`
}

// LoadUpdateState returns the last check, or the zero state.
func LoadUpdateState(dir string) UpdateState {
	var s UpdateState
	b, err := os.ReadFile(filepath.Join(dir, UpdateStateFile))
	if err == nil {
		json.Unmarshal(b, &s)
	}
	return s
}

// SaveUpdateState records a check.
func SaveUpdateState(dir string, s UpdateState) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, UpdateStateFile), b, 0o600)
}
