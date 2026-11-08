package engine

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
)

// Config backup (SET-005): the settings, providers, profiles,
// environments, rules, MCP servers and harness presets as one file,
// without secrets, and restored from it.

// ConfigRestore is what a restore did: the files written, and the
// providers whose API key is not in this computer's Keychain, to enter again.
type ConfigRestore struct {
	Files    []string `json:"files"`
	NeedKeys []string `json:"needKeys"`
	// NeedApproval counts what waits for the user's approval, which a backup does
	// not bring: hooks, agents, allow rules, environments and settings.
	NeedApproval int `json:"needApproval"`
	// Gone lists the deny rules and blocking hooks the backup's files left out.
	Gone       []string `json:"gone"`
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
	return env.WritePrivate(path, b)
}

// AddressChange is a place things are sent to that a restore adds or moves. From
// is empty for a new one.
type AddressChange struct {
	What string `json:"what"`
	From string `json:"from"`
	To   string `json:"to"`
}

// ConfigPreview is what restoring a backup would do. Nothing has changed yet.
type ConfigPreview struct {
	// Fingerprint is the backup's, for RestorePreviewed: a file changed since the
	// user read this is not what they agreed to.
	Fingerprint string              `json:"fingerprint"`
	Files       []config.BundleFile `json:"files"`
	// Gone lists the deny rules and blocking hooks that would no longer apply.
	Gone []string `json:"gone"`
	// NeedApproval is what would wait for approval, new with this backup.
	NeedApproval []PendingCommand `json:"needApproval"`
	Addresses    []AddressChange  `json:"addresses"`
	NeedKeys     []string         `json:"needKeys"`
}

func bundleFingerprint(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (e *Engine) readBundle(path string) ([]byte, error) {
	env, err := e.localEnv(filepath.Dir(path), nil)
	if err != nil {
		return nil, err
	}
	return env.FS().ReadFile(path)
}

// PreviewConfig says what restoring the backup at path would do, and changes
// nothing in the data folder.
func (e *Engine) PreviewConfig(ctx context.Context, path string) (ConfigPreview, error) {
	b, err := e.readBundle(path)
	if err != nil {
		return ConfigPreview{}, err
	}
	after, files, err := config.PreviewBundle(e.dataDir, b, builtin.ToolNames())
	if err != nil {
		return ConfigPreview{}, err
	}
	now := e.filesNow()
	afterGuards := guards(after)
	preview := ConfigPreview{Fingerprint: bundleFingerprint(b), Files: files, NeedApproval: []PendingCommand{}, Addresses: []AddressChange{},
		Gone: slices.DeleteFunc(guards(now), func(g string) bool { return slices.Contains(afterGuards, g) }), NeedKeys: e.missingKeys(after.Providers)}
	approvals := append(slices.Clone(now.Config.CommandApprovals), after.Config.CommandApprovals...)
	for _, c := range fileItems(after) {
		if !e.markIn(approvals, c.Fingerprint) {
			preview.NeedApproval = append(preview.NeedApproval, c)
		}
	}
	for _, o := range after.Optimizers {
		if o.Source != "" && !e.optimizerMarked(after.Config, o) {
			preview.NeedApproval = append(preview.NeedApproval, PendingCommand{Kind: pendingOptimizer, Source: o.Source, Runs: describeOptimizer(o), Fingerprint: o.Fingerprint()})
		}
	}
	// A restore clears the approvals of the servers in its mcp.yaml (MCP-005).
	if slices.ContainsFunc(files, func(f config.BundleFile) bool { return f.Path == config.MCPFileName }) {
		for _, s := range after.MCP {
			preview.NeedApproval = append(preview.NeedApproval, PendingCommand{Kind: pendingMCP, Source: config.MCPFileName, Runs: describeMCPServer(s), Fingerprint: s.Fingerprint()})
		}
	}
	for _, c := range after.Providers {
		i := slices.IndexFunc(now.Providers, func(p config.Provider) bool { return p.ID == c.ID })
		switch {
		case i < 0:
			preview.Addresses = append(preview.Addresses, AddressChange{What: c.Name, To: providerShown(c)})
		case keyAddress(now.Providers[i]) != keyAddress(c):
			preview.Addresses = append(preview.Addresses, AddressChange{What: c.Name, From: providerShown(now.Providers[i]), To: providerShown(c)})
		}
	}
	setting := func(what, from, to string) {
		if from != to && to != "" {
			preview.Addresses = append(preview.Addresses, AddressChange{What: what, From: from, To: to})
		}
	}
	setting("Web search", now.Config.Web.SearchURL, after.Config.Web.SearchURL)
	setting("MCP registry", now.Config.MCP.RegistryURL, after.Config.MCP.RegistryURL)
	return preview, nil
}

// describeMCPServer is what a restored server would run or connect to.
func describeMCPServer(s config.MCPServer) string {
	name := cmp.Or(s.Name, s.ID)
	if s.URL != "" {
		return name + " connects to " + s.URL
	}
	return name + " runs " + strings.Join(append([]string{s.Command}, s.Args...), " ")
}

// providerShown is a provider's address for reading.
func providerShown(c config.Provider) string {
	return cmp.Or(c.BaseURL, shippedAddress(c), "its default endpoint")
}

// missingKeys names the providers whose API key is not in this computer's Keychain.
func (e *Engine) missingKeys(cfgs []config.Provider) []string {
	out := []string{}
	for _, c := range cfgs {
		if c.APIKey == "" {
			continue
		}
		// Only this app's own item is looked at: a restored file naming another app's,
		// Claude Code's sign-in for one, must not get it read (HRN-008).
		ref, err := ownRef(c.APIKey, providerRef(c.ID))
		if err == nil {
			_, err = e.secrets.Get(ref)
		}
		if err != nil {
			out = append(out, c.Name)
		}
	}
	return out
}

// RestorePreviewed restores the backup at path if it is still the one previewed.
func (e *Engine) RestorePreviewed(ctx context.Context, path, fingerprint string) (ConfigRestore, error) {
	b, err := e.readBundle(path)
	if err != nil {
		return ConfigRestore{}, err
	}
	if bundleFingerprint(b) != fingerprint {
		return ConfigRestore{}, errors.New("the backup file changed after you looked at it; choose it again")
	}
	return e.restoreBundle(b)
}

// ImportConfig restores a config backup from path. The files it replaces
// are kept under backups/ in the data folder; the config is reloaded at
// once.
func (e *Engine) ImportConfig(ctx context.Context, path string) (ConfigRestore, error) {
	b, err := e.readBundle(path)
	if err != nil {
		return ConfigRestore{}, err
	}
	return e.restoreBundle(b)
}

func (e *Engine) restoreBundle(b []byte) (ConfigRestore, error) {
	approvals := e.Config().CommandApprovals
	before := guards(e.filesNow())
	written, err := config.ImportBundle(e.dataDir, b)
	if err != nil {
		return ConfigRestore{}, err
	}
	changed := make([]string, len(written))
	for i, rel := range written {
		changed[i] = filepath.FromSlash(rel)
	}
	e.reload(changed)
	// What this computer approved stays approved, for the files a backup does not hold
	// and for the same commands in those it does.
	if err := e.addApprovals(approvals); err != nil {
		return ConfigRestore{}, err
	}
	e.mu.Lock()
	cfgs := slices.Clone(e.providers.configs)
	e.mu.Unlock()
	res := ConfigRestore{Files: written, NeedKeys: e.missingKeys(cfgs), BackupNote: "The files it replaced are kept in the backups folder of the app's data folder."}
	res.NeedApproval = len(e.PendingCommands())
	after := guards(e.filesNow())
	res.Gone = slices.DeleteFunc(before, func(g string) bool { return slices.Contains(after, g) })
	if res.Gone == nil {
		res.Gone = []string{}
	}
	return res, nil
}
