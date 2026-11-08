package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/permission"
)

// EnvironmentsDir holds one YAML file per environment (ENV-002).
const EnvironmentsDir = "environments"

// Environment is one environment's settings. A local one runs on this Mac
// in the session's project folder; a docker one runs each session in its
// own container with the project bind-mounted (ENV-005). Several projects
// and sessions can share an environment.
type Environment struct {
	ID   string `yaml:"-" json:"id"` // the file name without .yaml
	Name string `yaml:"name" json:"name"`
	Kind string `yaml:"kind" json:"kind"`
	// Env is added to the environment of every command.
	Env map[string]string `yaml:"env,omitempty" json:"env"`
	// Secrets maps a variable name to a keychain:// reference; values are
	// resolved when a command starts (ENV-004).
	Secrets map[string]string `yaml:"secrets,omitempty" json:"-"`
	// DisabledTools are not offered to the model in this environment.
	DisabledTools []string `yaml:"disabledTools,omitempty" json:"disabledTools"`
	// DefaultMode is the permission mode new sessions start in.
	DefaultMode string `yaml:"defaultMode,omitempty" json:"defaultMode"`
	// Rules apply to every session in the environment, between the global
	// and the profile rules (SEC-002).
	Rules []permission.Rule `yaml:"rules,omitempty" json:"rules"`
	// Docker: the image to run, or Devcontainer to take image or Dockerfile
	// and setup from the project's .devcontainer/devcontainer.json; Setup
	// runs once after a container is created and Health before every
	// session start (ENV-005, ENV-007).
	Image        string `yaml:"image,omitempty" json:"image"`
	Devcontainer bool   `yaml:"devcontainer,omitempty" json:"devcontainer"`
	Setup        string `yaml:"setup,omitempty" json:"setup"`
	Health       string `yaml:"health,omitempty" json:"health"`
	// Sandbox confines a local environment's commands to the project (ENV-009).
	Sandbox *Sandbox `yaml:"sandbox,omitempty" json:"sandbox"`
	// SSH: the machine, as ssh knows it, and the folder there that holds each
	// session's working copy; Setup and Health run there too (ENV-008).
	Host      string `yaml:"host,omitempty" json:"host"`
	RemoteDir string `yaml:"remoteDir,omitempty" json:"remoteDir"`
	// SSHAgent false keeps SSH_AUTH_SOCK from the commands, agents and terminals of
	// a local environment, so they cannot use the user's SSH keys (git over SSH
	// then needs another way in). Unset is true. Only taking access away, it is not
	// part of what an approval covers.
	SSHAgent *bool `yaml:"sshAgent,omitempty" json:"sshAgent"`
}

// ShareSSHAgent reports whether commands may use the user's SSH agent.
func (e Environment) ShareSSHAgent() bool { return e.SSHAgent == nil || *e.SSHAgent }

// Fingerprint identifies what the environment runs and lets commands reach. Its
// name and disabled tools change neither, and each of its rules has its own.
func (e Environment) Fingerprint() string {
	// Which machine and folder is part of what the user approves. Only for this kind, so
	// that no other environment's approval ends for being stored differently.
	if e.Kind == "ssh" {
		return commandFingerprint("environment-ssh", struct {
			ID, Kind, DefaultMode, Setup, Health, Host, RemoteDir string
			Env, Secrets                                          map[string]string
		}{e.ID, e.Kind, e.DefaultMode, e.Setup, e.Health, e.Host, e.RemoteDir, e.Env, e.Secrets})
	}
	return commandFingerprint("environment", struct {
		ID, Kind, DefaultMode, Image, Setup, Health string
		Devcontainer                                bool
		Env, Secrets                                map[string]string
		Sandbox                                     *Sandbox
	}{e.ID, e.Kind, e.DefaultMode, e.Image, e.Setup, e.Health, e.Devcontainer, e.Env, e.Secrets, e.Sandbox})
}

// Sandbox is how a local environment's commands are confined.
type Sandbox struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Network is localhost (the default), none, or hosts: localhost and the
	// hosts listed in Hosts (ENV-010).
	Network string `yaml:"network,omitempty" json:"network"`
	// Hosts are names such as registry.npmjs.org or *.github.com, with
	// :port for a port other than 80 and 443.
	Hosts []string `yaml:"hosts,omitempty" json:"hosts"`
	// Read and Write are more folders the commands can reach, absolute or
	// under ~/. The never-readable locations stay closed whatever these say.
	Read  []string `yaml:"read,omitempty" json:"read"`
	Write []string `yaml:"write,omitempty" json:"write"`
}

// SandboxOn reports whether the environment confines its commands.
func (e Environment) SandboxOn() bool { return e.Sandbox != nil && e.Sandbox.Enabled }

// sandboxHost is a name with at least two labels, or a wildcard over one; a
// wildcard alone would be no list at all.
var sandboxHost = regexp.MustCompile(`^(\*\.)?[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+(:[0-9]{1,5})?$`)

var varName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Validate reports settings that cannot work.
func (e Environment) Validate() error {
	switch {
	case !idPattern.MatchString(e.ID):
		return fmt.Errorf("id %q must be lowercase letters, digits and dashes", e.ID)
	case strings.TrimSpace(e.Name) == "":
		return errors.New("name is required")
	case e.Kind != "local" && e.Kind != "docker" && e.Kind != "ssh":
		return fmt.Errorf("kind %q must be local, docker or ssh", e.Kind)
	case e.Kind == "ssh" && !environment.ValidSSHHost(e.Host):
		return fmt.Errorf("host %q must be a name from ~/.ssh/config or [user@]host", e.Host)
	case e.Kind == "ssh" && e.RemoteDir != "" && !environment.ValidRemoteDir(e.RemoteDir):
		return fmt.Errorf("remoteDir %q must be a plain path, with no spaces, .. or shell characters", e.RemoteDir)
	case e.Kind == "docker" && strings.TrimSpace(e.Image) == "" && !e.Devcontainer:
		return errors.New("a docker environment needs an image, or devcontainer: true")
	case e.DefaultMode != "" && (!permission.Mode(e.DefaultMode).Valid() || permission.Mode(e.DefaultMode).Unattended()):
		return fmt.Errorf("defaultMode %q must be ask, accept_edits or plan", e.DefaultMode)
	}
	if e.Sandbox != nil {
		switch {
		case e.Kind != "local" && e.Sandbox.Enabled:
			return errors.New("sandbox: only a local environment has one; a docker environment has its container")
		case e.Sandbox.Network != "" && e.Sandbox.Network != "localhost" && e.Sandbox.Network != "none" && e.Sandbox.Network != "hosts":
			return fmt.Errorf("sandbox: network %q must be localhost, none or hosts", e.Sandbox.Network)
		case e.Sandbox.Network == "hosts" && len(e.Sandbox.Hosts) == 0:
			return errors.New("sandbox: network hosts needs at least one host")
		}
		for _, h := range e.Sandbox.Hosts {
			if !sandboxHost.MatchString(h) {
				return fmt.Errorf("sandbox: %q is not a host name such as registry.npmjs.org or *.github.com", h)
			}
		}
		for _, p := range slices.Concat(e.Sandbox.Read, e.Sandbox.Write) {
			if p == "/" || p == "~" || p == "~/" || (!strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "~/")) {
				return fmt.Errorf("sandbox: %q must be a folder given absolute or under ~/, not all of it", p)
			}
		}
	}
	for k := range e.Env {
		if !varName.MatchString(k) {
			return fmt.Errorf("env: %q is not a valid variable name", k)
		}
	}
	for i, r := range e.Rules {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
	}
	for k, ref := range e.Secrets {
		if !varName.MatchString(k) {
			return fmt.Errorf("secrets: %q is not a valid variable name", k)
		}
		if _, ok := e.Env[k]; ok {
			return fmt.Errorf("%s is both a variable and a secret", k)
		}
		if !strings.HasPrefix(ref, "keychain://") {
			return fmt.Errorf("secrets: %s must be a keychain:// reference, never a value", k)
		}
	}
	return nil
}

func environmentsDir(dir string) string { return filepath.Join(dir, EnvironmentsDir) }

// LoadEnvironments reads every environments/*.yaml in dir, sorted by ID.
// Files with problems are skipped and reported.
func LoadEnvironments(dir string) ([]Environment, []Issue, error) {
	entries, err := os.ReadDir(environmentsDir(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}
	var out []Environment
	var issues []Issue
	for _, de := range entries {
		id, ok := strings.CutSuffix(de.Name(), ".yaml")
		if !ok || de.IsDir() {
			continue
		}
		path := filepath.Join(environmentsDir(dir), de.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			issues = append(issues, Issue{File: path, Message: err.Error()})
			continue
		}
		var e Environment
		dec := yaml.NewDecoder(bytes.NewReader(b))
		dec.KnownFields(true)
		if err := dec.Decode(&e); err != nil && !errors.Is(err, io.EOF) {
			issues = append(issues, Issue{File: path, Message: err.Error()})
			continue
		}
		e.ID = id
		if e.Kind == "" {
			e.Kind = "local"
		}
		for i := range e.Rules {
			e.Rules[i].Scope, e.Rules[i].Source = permission.ScopeEnvironment, filepath.Join(EnvironmentsDir, id+".yaml")
		}
		if err := e.Validate(); err != nil {
			issues = append(issues, Issue{File: path, Message: err.Error()})
			continue
		}
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b Environment) int { return strings.Compare(a.ID, b.ID) })
	return out, issues, nil
}

// SaveEnvironment writes environments/<id>.yaml.
func SaveEnvironment(dir string, e Environment) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(environmentsDir(dir), 0o700); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	b, err := yaml.Marshal(e)
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(environmentsDir(dir), e.ID+".yaml"), b)
}

// DeleteEnvironment removes environments/<id>.yaml.
func DeleteEnvironment(dir, id string) error {
	return removeYAML(environmentsDir(dir), "environment", id)
}

// removeYAML removes <id>.yaml from a folder; a missing one is fine.
func removeYAML(dir, kind, id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("bad %s id %q", kind, id)
	}
	err := os.Remove(filepath.Join(dir, id+".yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
