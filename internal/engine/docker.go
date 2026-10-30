package engine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// Docker environments (ENV-005, ENV-006, ENV-007): each session of a docker
// environment runs in its own container, created when the session first
// runs from the environment's image or the project's devcontainer.json,
// with the session's folder bind-mounted at the same path. Commands and
// external agents run inside it; files are read and written on the host;
// the app's own git commands stay on the host.

// dockerStartTimeout covers pulling or building an image and the setup
// command on a container's first start.
const dockerStartTimeout = 20 * time.Minute

// containerName is a session's container.
func containerName(sid string) string {
	return "freeloop-" + strings.ReplaceAll(sid, "_", "-")
}

var devcontainerFiles = []string{".devcontainer/devcontainer.json", ".devcontainer.json"}

// dockerEnv builds and starts the container a session of envCfg runs in,
// and records what it did and whether the environment is healthy.
func (e *Engine) dockerEnv(ctx context.Context, ses session.Session, project session.Project, envCfg config.Environment, vars map[string]string, host *environment.Local) (*environment.Docker, error) {
	opts := environment.DockerOptions{
		ID: envCfg.ID, Container: containerName(ses.ID), Root: ses.WorkRoot(), Image: envCfg.Image,
		Env: maps.Clone(vars), Setup: envCfg.Setup, Health: envCfg.Health, Guard: e.broker.Policy().Guard(ses.WorkRoot()),
		// What the app reads from the project and may run on the host
		// stays out of the container's reach.
		ReadOnly: []string{filepath.Join(ses.WorkRoot(), ProjectDir) + "/"},
	}
	if envCfg.Devcontainer {
		// A devcontainer.json is project configuration (SEC-006).
		if !project.Trusted {
			return nil, errors.New("trust the project before using its devcontainer.json")
		}
		var dc config.Devcontainer
		found := false
		for _, f := range devcontainerFiles {
			b, err := host.FS().ReadFile(f)
			if err != nil {
				continue
			}
			if dc, err = config.ParseDevcontainer(f, b); err != nil {
				return nil, err
			}
			found = true
			ro := filepath.Join(host.Root(), filepath.FromSlash(f))
			if strings.Contains(f, "/") {
				ro = filepath.Dir(ro)
			}
			opts.ReadOnly = append(opts.ReadOnly, ro)
			break
		}
		if !found {
			return nil, errors.New("the environment uses the project's devcontainer.json, and the project has none")
		}
		opts.Image = dc.Image
		if dc.Dockerfile != "" {
			opts.Image = ""
			opts.Dockerfile = filepath.Join(host.Root(), filepath.FromSlash(dc.Dockerfile))
			opts.BuildContext = filepath.Join(host.Root(), filepath.FromSlash(dc.Context))
		}
		if opts.Setup == "" {
			opts.Setup = dc.PostCreate
		}
		// The project file's variables go on the command line, never into
		// the host docker CLI's environment.
		opts.PlainEnv = dc.Env
	}
	d, err := environment.NewDocker(opts)
	if err != nil {
		return nil, err
	}
	sctx, cancel := context.WithTimeout(ctx, dockerStartTimeout)
	defer cancel()
	did, err := d.Start(sctx)
	health := map[string]string{"status": "healthy", "container": opts.Container}
	if did != "" {
		health["detail"] = did
	}
	if err != nil {
		health["status"], health["detail"] = "unhealthy", err.Error()
		e.emit(ses.ID, event.EnvHealth, health)
		return nil, fmt.Errorf("the %s environment cannot start: %w", envCfg.Name, err)
	}
	e.emit(ses.ID, event.EnvHealth, health)
	return d, nil
}
