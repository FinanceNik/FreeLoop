package engine

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// DefaultEnvironmentID is the environment every project starts with. It
// exists without a file; saving it writes environments/local.yaml.
const DefaultEnvironmentID = "local"

func defaultEnvironment() config.Environment {
	return config.Environment{ID: DefaultEnvironmentID, Name: "Local", Kind: "local"}
}

// EnvironmentInfo is an environment as the UI sees it: secret names, never
// secret values.
type EnvironmentInfo struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Kind          string            `json:"kind"`
	Env           map[string]string `json:"env"`
	SecretNames   []string          `json:"secretNames"`
	DisabledTools []string          `json:"disabledTools"`
	DefaultMode   string            `json:"defaultMode"`
}

func envInfo(c config.Environment) EnvironmentInfo {
	names := slices.Sorted(maps.Keys(c.Secrets))
	env := c.Env
	if env == nil {
		env = map[string]string{}
	}
	tools := c.DisabledTools
	if tools == nil {
		tools = []string{}
	}
	return EnvironmentInfo{ID: c.ID, Name: c.Name, Kind: c.Kind, Env: env, SecretNames: names, DisabledTools: tools, DefaultMode: c.DefaultMode}
}

func (e *Engine) loadEnvironments() {
	envs, issues, err := config.LoadEnvironments(e.dataDir)
	if err != nil {
		issues = append(issues, config.Issue{File: config.EnvironmentsDir, Message: err.Error()})
	}
	if !slices.ContainsFunc(envs, func(c config.Environment) bool { return c.ID == DefaultEnvironmentID }) {
		envs = append([]config.Environment{defaultEnvironment()}, envs...)
	}
	e.mu.Lock()
	e.envs = envs
	e.issues[issueEnvironments] = issues
	e.mu.Unlock()
}

func (e *Engine) environment(id string) (config.Environment, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if i := slices.IndexFunc(e.envs, func(c config.Environment) bool { return c.ID == id }); i >= 0 {
		return e.envs[i], nil
	}
	return config.Environment{}, fmt.Errorf("no environment %q", id)
}

// ListEnvironments returns every environment, the default first.
func (e *Engine) ListEnvironments() []EnvironmentInfo {
	e.mu.Lock()
	envs := slices.Clone(e.envs)
	e.mu.Unlock()
	out := make([]EnvironmentInfo, 0, len(envs))
	for _, c := range envs {
		out = append(out, envInfo(c))
	}
	return out
}

// SaveEnvironmentRequest adds or updates an environment. Secrets holds new
// values by name, written to the Keychain; a name with an empty value keeps
// the stored secret. RemoveSecrets deletes secrets by name.
type SaveEnvironmentRequest struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Env           map[string]string `json:"env"`
	Secrets       map[string]string `json:"secrets"`
	RemoveSecrets []string          `json:"removeSecrets"`
	DisabledTools []string          `json:"disabledTools"`
	DefaultMode   string            `json:"defaultMode"`
}

func secretRef(envID, name string) secrets.Ref {
	return secrets.Ref{Service: "freeloop", Account: "environments/" + envID + "/" + name}
}

// SaveEnvironment adds (empty ID) or updates an environment (ENV-002).
func (e *Engine) SaveEnvironment(req SaveEnvironmentRequest) (EnvironmentInfo, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return EnvironmentInfo{}, errors.New("an environment needs a name")
	}
	e.mu.Lock()
	envs := slices.Clone(e.envs)
	e.mu.Unlock()

	var cur config.Environment
	if req.ID == "" {
		base := strings.Trim(slugChars.ReplaceAllString(strings.ToLower(req.Name), "-"), "-")
		if base == "" {
			base = "environment"
		}
		req.ID = base
		for n := 2; slices.ContainsFunc(envs, func(c config.Environment) bool { return c.ID == req.ID }); n++ {
			req.ID = fmt.Sprintf("%s-%d", base, n)
		}
		cur = config.Environment{ID: req.ID, Kind: "local"}
	} else {
		i := slices.IndexFunc(envs, func(c config.Environment) bool { return c.ID == req.ID })
		if i < 0 {
			return EnvironmentInfo{}, fmt.Errorf("no environment %q", req.ID)
		}
		cur = envs[i]
	}

	next := config.Environment{ID: cur.ID, Name: req.Name, Kind: cur.Kind, DefaultMode: req.DefaultMode, Secrets: map[string]string{}}
	if len(req.Env) > 0 {
		next.Env = map[string]string{}
		for k, v := range req.Env {
			next.Env[strings.TrimSpace(k)] = v
		}
	}
	if len(req.DisabledTools) > 0 {
		next.DisabledTools = slices.Sorted(slices.Values(slices.Compact(slices.Sorted(slices.Values(req.DisabledTools)))))
	}
	for k, v := range cur.Secrets {
		if !slices.Contains(req.RemoveSecrets, k) {
			next.Secrets[k] = v
		}
	}
	// Validate before touching the Keychain.
	for name := range req.Secrets {
		next.Secrets[strings.TrimSpace(name)] = secretRef(cur.ID, strings.TrimSpace(name)).String()
	}
	if len(next.Secrets) == 0 {
		next.Secrets = nil
	}
	if err := next.Validate(); err != nil {
		return EnvironmentInfo{}, err
	}
	for name, value := range req.Secrets {
		name = strings.TrimSpace(name)
		if value == "" {
			if _, had := cur.Secrets[name]; !had {
				return EnvironmentInfo{}, fmt.Errorf("secret %s needs a value", name)
			}
			continue
		}
		if err := e.secrets.Set(secretRef(cur.ID, name), value); err != nil {
			return EnvironmentInfo{}, fmt.Errorf("store secret %s in the Keychain: %w", name, err)
		}
		e.remember(value)
	}
	for _, name := range req.RemoveSecrets {
		if _, kept := next.Secrets[name]; !kept {
			if err := e.secrets.Delete(secretRef(cur.ID, name)); err != nil && !errors.Is(err, secrets.ErrNotFound) {
				return EnvironmentInfo{}, err
			}
		}
	}
	if err := config.SaveEnvironment(e.dataDir, next); err != nil {
		return EnvironmentInfo{}, err
	}
	e.loadEnvironments()
	return envInfo(next), nil
}

// DeleteEnvironment removes an environment and its secrets. Deleting the
// default environment resets it.
func (e *Engine) DeleteEnvironment(id string) error {
	c, err := e.environment(id)
	if err != nil {
		return err
	}
	for name := range c.Secrets {
		if err := e.secrets.Delete(secretRef(id, name)); err != nil && !errors.Is(err, secrets.ErrNotFound) {
			return err
		}
	}
	if err := config.DeleteEnvironment(e.dataDir, id); err != nil {
		return err
	}
	e.loadEnvironments()
	return nil
}

// resolveEnv returns the variables a session's processes get: the
// environment's variables plus its secrets, read from the Keychain now
// (ENV-004). Secret values are added to the redaction list.
func (e *Engine) resolveEnv(c config.Environment) (map[string]string, error) {
	vars := maps.Clone(c.Env)
	if vars == nil {
		vars = map[string]string{}
	}
	for name, ref := range c.Secrets {
		r, err := secrets.ParseRef(ref)
		if err != nil {
			return nil, fmt.Errorf("environment %s: secret %s: %w", c.Name, name, err)
		}
		v, err := e.secrets.Get(r)
		if err != nil {
			return nil, fmt.Errorf("environment %s: read secret %s from the Keychain: %w", c.Name, name, err)
		}
		e.remember(v)
		vars[name] = v
	}
	return vars, nil
}
