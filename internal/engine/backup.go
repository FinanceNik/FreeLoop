package engine

import (
	"context"
	"errors"
	"path/filepath"
	"slices"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// Config backup (SET-005): the settings, providers, profiles,
// environments, rules, MCP servers and harness presets as one file,
// without secrets, and restored from it.

// ConfigRestore is what a restore did: the files written, and the
// providers whose API key is not in this Mac's Keychain, to enter again.
type ConfigRestore struct {
	Files      []string `json:"files"`
	NeedKeys   []string `json:"needKeys"`
	BackupNote string   `json:"backupNote"`
}

// ExportConfig writes a config backup to path, through a local
// environment like every file the app writes.
func (e *Engine) ExportConfig(ctx context.Context, path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("the backup needs an absolute path")
	}
	b, err := config.ExportBundle(e.dataDir)
	if err != nil {
		return err
	}
	env, err := e.localEnv(filepath.Dir(path), nil)
	if err != nil {
		return err
	}
	return env.FS().WriteFile(path, b)
}

// ImportConfig restores a config backup from path. The files it replaces
// are kept under backups/ in the data folder; the config is reloaded at
// once.
func (e *Engine) ImportConfig(ctx context.Context, path string) (ConfigRestore, error) {
	env, err := e.localEnv(filepath.Dir(path), nil)
	if err != nil {
		return ConfigRestore{}, err
	}
	b, err := env.FS().ReadFile(path)
	if err != nil {
		return ConfigRestore{}, err
	}
	written, err := config.ImportBundle(e.dataDir, b)
	if err != nil {
		return ConfigRestore{}, err
	}
	changed := make([]string, len(written))
	for i, rel := range written {
		changed[i] = filepath.FromSlash(rel)
	}
	e.reload(changed)
	e.mu.Lock()
	cfgs := slices.Clone(e.providers.configs)
	e.mu.Unlock()
	res := ConfigRestore{Files: written, NeedKeys: []string{}, BackupNote: "The files it replaced are kept in the backups folder of the app's data folder."}
	for _, c := range cfgs {
		if c.APIKey == "" {
			continue
		}
		ref, err := secrets.ParseRef(c.APIKey)
		if err == nil {
			_, err = e.secrets.Get(ref)
		}
		if err != nil {
			res.NeedKeys = append(res.NeedKeys, c.Name)
		}
	}
	return res, nil
}
