package engine

import (
	"context"
	"log"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// watch polls the hand-editable config files and reloads the ones that
// changed (SET-001). Polling needs no dependency and meets the two-second
// target; the files are few and small. Problems in an edited file are
// reported through ConfigIssues as at startup, never as a crash, and the
// app's own saves pass through here too, which is harmless.
func (e *Engine) watch(interval time.Duration) {
	defer e.watching.Done()
	t := time.NewTicker(interval)
	defer t.Stop()
	last := config.Fingerprint(e.dataDir)
	for {
		select {
		case <-e.quit:
			return
		case <-t.C:
		}
		e.watchAgents()
		e.stopIdle(time.Now())
		now := config.Fingerprint(e.dataDir)
		changed := config.Changed(last, now)
		last = now
		if len(changed) == 0 {
			continue
		}
		slices.Sort(changed)
		e.reload(changed)
		if e.onChange != nil {
			e.onChange(ConfigChange{Files: changed})
		}
	}
}

// reload rereads the config the changed files hold. Running sessions keep
// the provider and environment they started with; new sessions and the next
// permission check see the new settings.
func (e *Engine) reload(changed []string) {
	envs, profiles, styles := false, false, false
	for _, f := range changed {
		switch {
		case f == config.FileName:
			cfg, issues, err := config.Load(e.dataDir)
			if err != nil {
				issues = append(issues, config.Issue{File: config.FileName, Message: err.Error(), Unreadable: true})
			}
			e.mu.Lock()
			// A file that cannot be read keeps the settings in force, so a
			// half-saved edit does not turn off local-only mode or hooks.
			if !config.Unreadable(issues) {
				e.cfg = cfg
			}
			localOnly := e.cfg.Network.LocalOnly
			e.issues[issueConfig] = issues
			e.mu.Unlock()
			netguard.SetLocalOnly(localOnly)
			e.dropBypass(context.Background())
		case f == config.ProvidersFileName:
			if err := e.loadProviders(); err != nil {
				e.setIssues(issueProviders, []config.Issue{{File: config.ProvidersFileName, Message: err.Error()}})
			}
			e.learnKeyAddresses()
			e.checkKeyAddresses()
		case f == config.RulesFileName:
			e.rules.reload()
		case f == config.MCPFileName:
			e.loadMCP()
		case f == config.ColorsFileName:
			e.loadColors()
		case strings.HasPrefix(f, config.StylesDir+string(filepath.Separator)):
			styles = true
		case strings.HasPrefix(f, config.EnvironmentsDir+string(filepath.Separator)):
			envs = true
		case strings.HasPrefix(f, config.ProfilesDir+string(filepath.Separator)):
			profiles = true
		case strings.HasPrefix(f, config.OptimizersDir+string(filepath.Separator)):
			e.loadOptimizers()
		case strings.HasPrefix(f, config.WorkflowsDir+string(filepath.Separator)):
			e.loadWorkflows()
		case strings.HasPrefix(f, config.PresetsDir+string(filepath.Separator)):
			e.loadPresets()
			profiles = true
		default:
			log.Printf("engine: no reload for %s", f)
		}
	}
	if styles {
		e.loadStyles()
	}
	if envs {
		e.loadEnvironments()
	}
	if profiles {
		e.loadProfiles()
	}
}
