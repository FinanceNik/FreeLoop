package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// The in-app demo (GUI-045): a scratch git project beside the app's data folder and a
// session on the scripted tour, which meets each answer of the permission policy in
// one turn. No key and no network are needed.

// demoTag marks the demo session.
const demoTag = "demo"

const demoPrompt = "Show me what Freeloop does."

// demoDir is the demo project's folder. It is beside the data folder, not in it: the
// data folder is a protected location (SEC-013), where no agent may read or write, and
// the tour has to read and write in its project.
func (e *Engine) demoDir() string { return e.dataDir + "-demo" }

// removeDemoDir deletes the demo project, whatever is in it.
func (e *Engine) removeDemoDir() error {
	return environment.RemoveOwned(filepath.Dir(e.dataDir), e.demoDir())
}

// demoSession returns the demo session, if there is one.
func (e *Engine) demoSession(ctx context.Context) (session.Session, bool) {
	list, err := e.store.ListSessions(ctx)
	if err != nil {
		return session.Session{}, false
	}
	for _, s := range list {
		if slices.Contains(s.Tags, demoTag) {
			return s, true
		}
	}
	return session.Session{}, false
}

// StartDemo returns the demo session, making it first when there is none: a scratch
// git project and a session on the scripted tour, whose one turn starts at once.
func (e *Engine) StartDemo(ctx context.Context) (session.Session, error) {
	if s, ok := e.demoSession(ctx); ok {
		return s, nil
	}
	root := e.demoDir()
	if err := e.removeDemoDir(); err != nil {
		return session.Session{}, err
	}
	if err := e.makeDemoProject(ctx, root); err != nil {
		return session.Session{}, err
	}
	if _, err := e.SetProjectTrust(ctx, root, true); err != nil {
		return session.Session{}, err
	}
	ses, err := e.CreateSession(ctx, CreateSessionRequest{
		Title: "Demo: your policy at work", ProjectRoot: root,
		ProviderID: TourProviderID, Model: "tour", Mode: string(permission.ModeAsk),
	})
	if err != nil {
		return session.Session{}, err
	}
	if _, err := e.SetTags(ctx, ses.ID, []string{demoTag}); err != nil {
		return session.Session{}, err
	}
	if err := e.AddSessionRule(ses.ID, permission.Rule{Effect: permission.Deny, Tool: "shell", Command: "git push *"}); err != nil {
		return session.Session{}, err
	}
	if _, err := e.Prompt(ctx, ses.ID, demoPrompt); err != nil {
		return session.Session{}, err
	}
	return e.store.GetSession(ctx, ses.ID)
}

// makeDemoProject writes the scratch project and commits it.
func (e *Engine) makeDemoProject(ctx context.Context, root string) error {
	if err := environment.PrivateCache(root); err != nil {
		return err
	}
	env, err := e.localEnv(root, nil)
	if err != nil {
		return err
	}
	files := map[string]string{
		"README.md": "# Demo project\n\nThis is Freeloop's demo project. It is a scratch folder next to the app's data folder, made for the tour and deleted by \"Remove demo\".\n",
		"main.go":   "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello from the demo project\")\n}\n",
		".env":      "DEMO_TOKEN=not-a-real-token\n",
	}
	for name, content := range files {
		if err := env.FS().WriteFile(filepath.Join(root, name), []byte(content)); err != nil {
			return fmt.Errorf("demo project: %w", err)
		}
	}
	author := map[string]string{
		"GIT_AUTHOR_NAME": "Freeloop demo", "GIT_AUTHOR_EMAIL": "demo@freeloop.invalid",
		"GIT_COMMITTER_NAME": "Freeloop demo", "GIT_COMMITTER_EMAIL": "demo@freeloop.invalid",
	}
	for _, argv := range [][]string{
		{"git", "init", "-q", "-b", "main"},
		{"git", "add", "."},
		{"git", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "Start the demo project"},
	} {
		res, err := env.Exec(ctx, environment.ExecSpec{Argv: argv, Env: gitEnv(author), MaxOutput: 1 << 16})
		if err != nil {
			return fmt.Errorf("demo project: %w", err)
		}
		if res.ExitCode != 0 {
			return fmt.Errorf("demo project: %v failed: %s", argv[1:], res.Output)
		}
	}
	return nil
}

// RemoveDemo deletes the demo session, the scratch project and its record.
func (e *Engine) RemoveDemo(ctx context.Context, sid string) error {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return err
	}
	if !slices.Contains(ses.Tags, demoTag) {
		return errors.New("that is not the demo session")
	}
	if err := e.DeleteSession(ctx, sid); err != nil {
		return err
	}
	if err := e.removeDemoDir(); err != nil {
		return err
	}
	// By the session's own record: a project's root is stored with symlinks resolved,
	// which the data folder's path may not be (a temporary folder on macOS).
	if ses.ProjectID != "" {
		if err := e.store.DeleteProject(ctx, ses.ProjectID); err != nil {
			return err
		}
	}
	return nil
}
