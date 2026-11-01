package engine

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// One task on several profiles, compared (HRN-016). The same first message
// goes to two to four profiles, each in a session with its own worktree,
// so the runs never touch each other's files; a tag groups them. The
// compare page shows what each did, and keeping one archives the others,
// whose worktrees go while their branches stay.

// CompareProfile is one run's profile, with the provider and model a
// built-in profile needs.
type CompareProfile struct {
	ProfileID  string `json:"profileId"`
	ProviderID string `json:"providerId"`
	Model      string `json:"model"`
	// Label names the run in its title; the profile's ID when empty.
	Label string `json:"label"`
}

// CompareRequest starts a comparison.
type CompareRequest struct {
	Title         string           `json:"title"`
	ProjectRoot   string           `json:"projectRoot"`
	EnvironmentID string           `json:"environmentId"`
	Mode          string           `json:"mode"`
	Profiles      []CompareProfile `json:"profiles"`
	Prompt        string           `json:"prompt"`
}

// CompareTagPrefix starts the tag that groups a comparison's runs.
const CompareTagPrefix = "compare-"

// CreateCompare makes one worktree session per profile, tags them as one
// comparison and sends each the same first message. It returns the tag.
func (e *Engine) CreateCompare(ctx context.Context, req CompareRequest) (string, error) {
	prompt := strings.TrimSpace(req.Prompt)
	switch {
	case len(req.Profiles) < 2 || len(req.Profiles) > 4:
		return "", errors.New("compare two to four profiles")
	case prompt == "":
		return "", errors.New("write the task the runs are compared on")
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = titleFrom(prompt)
	}
	tag := CompareTagPrefix + strconv.FormatInt(time.Now().UnixMilli()%1_000_000_000, 36)
	var made []session.Session
	for _, p := range req.Profiles {
		label := cmp.Or(strings.TrimSpace(p.Label), p.ProfileID)
		ses, err := e.CreateSession(ctx, CreateSessionRequest{
			Title: fmt.Sprintf("%s · %s", title, label), ProjectRoot: req.ProjectRoot, EnvironmentID: req.EnvironmentID,
			ProfileID: p.ProfileID, ProviderID: p.ProviderID, Model: p.Model, Mode: req.Mode, Worktree: true,
		})
		if err != nil {
			// Runs already made stay, so their worktrees are not lost
			// silently; the user can archive them.
			if len(made) > 0 {
				return tag, fmt.Errorf("%s: %w (the runs already started are tagged %s)", label, err, tag)
			}
			return "", fmt.Errorf("%s: %w", label, err)
		}
		e.SetTags(ctx, ses.ID, []string{tag})
		made = append(made, ses)
	}
	for _, ses := range made {
		if _, err := e.Prompt(ctx, ses.ID, prompt); err != nil {
			return tag, err
		}
	}
	return tag, nil
}

// CompareRun is one run of a comparison as the compare page shows it.
type CompareRun struct {
	Session session.Session `json:"session"`
	// Files, Added and Deleted count the changes against the base commit,
	// committed or not; untracked files count as files.
	Files     int     `json:"files"`
	Added     int     `json:"added"`
	Deleted   int     `json:"deleted"`
	Tokens    int64   `json:"tokens"`
	Cost      float64 `json:"cost"`
	Priced    bool    `json:"priced"`
	LastReply string  `json:"lastReply"`
}

// Compare returns the runs of the comparison tag names.
func (e *Engine) Compare(ctx context.Context, tag string) ([]CompareRun, error) {
	if !strings.HasPrefix(tag, CompareTagPrefix) {
		return nil, errors.New("not a comparison")
	}
	all, err := e.store.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	var runs []CompareRun
	since := ""
	for _, s := range all {
		if slices.Contains(s.Tags, tag) {
			runs = append(runs, CompareRun{Session: s})
			if d := s.CreatedAt.In(time.Local).Format(time.DateOnly); since == "" || d < since {
				since = d
			}
		}
	}
	slices.SortFunc(runs, func(a, b CompareRun) int { return strings.Compare(a.Session.Title, b.Session.Title) })
	usage, _ := e.store.UsageRowsSince(ctx, since)
	prices := e.Config().Prices
	for i := range runs {
		r := &runs[i]
		for _, u := range usage {
			if u.SessionID != r.Session.ID {
				continue
			}
			r.Tokens += u.InputTokens + u.OutputTokens + u.CacheReadTokens + u.CacheWriteTokens
			if p, ok := config.FindPrice(prices, u.ProviderID, u.Model); ok {
				r.Cost += p.Cost(u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens)
				r.Priced = true
			}
		}
		r.Files, r.Added, r.Deleted = e.diffStat(ctx, r.Session)
		r.LastReply = e.lastReply(ctx, r.Session.ID)
	}
	return runs, nil
}

// maxCountedNew bounds the untracked files whose lines are counted.
const maxCountedNew = 200

// diffStat counts a session's changes against its base commit.
func (e *Engine) diffStat(ctx context.Context, ses session.Session) (files, added, deleted int) {
	if ses.BaseCommit == "" || ses.Status == session.StatusArchived || !e.trusted(ses.ProjectID) {
		return
	}
	env, _, err := e.sessionGit(ctx, ses.ID)
	if err != nil {
		return
	}
	if blocked(ctx, env) != "" {
		return
	}
	if res, err := gitRun(ctx, env, 30*time.Second, "diff", "--no-ext-diff", "--no-textconv", "--ignore-submodules=dirty", "--numstat", ses.BaseCommit, "--"); err == nil && res.ExitCode == 0 {
		for _, line := range strings.Split(strings.TrimSpace(res.Output), "\n") {
			f := strings.Fields(line)
			if len(f) < 3 {
				continue
			}
			files++
			a, _ := strconv.Atoi(f[0]) // "-" for a binary file
			d, _ := strconv.Atoi(f[1])
			added += a
			deleted += d
		}
	}
	if res, err := gitRun(ctx, env, 30*time.Second, "ls-files", "--others", "--exclude-standard"); err == nil && res.ExitCode == 0 {
		for i, line := range strings.Split(strings.TrimSpace(res.Output), "\n") {
			if line == "" {
				continue
			}
			files++
			// A new file's lines, for the first files only.
			if i >= maxCountedNew {
				continue
			}
			if res, err := gitRun(ctx, env, 30*time.Second, "diff", "--no-index", "--no-ext-diff", "--no-textconv", "--numstat", "--", "/dev/null", line); err == nil {
				if f := strings.Fields(res.Output); len(f) >= 3 {
					a, _ := strconv.Atoi(f[0])
					added += a
				}
			}
		}
	}
	return
}

// lastReply is the end of the agent's last message in a session.
func (e *Engine) lastReply(ctx context.Context, sid string) string {
	return tail(e.fullReply(ctx, sid), 800)
}

// fullReply is the agent's last message in a session.
func (e *Engine) fullReply(ctx context.Context, sid string) string {
	evs, err := e.store.Events(ctx, sid, 0)
	if err != nil {
		return ""
	}
	var cur, last strings.Builder
	inReply := false
	for _, ev := range evs {
		if ev.Type == event.AgentMessageChunk {
			if !inReply {
				cur.Reset()
				inReply = true
			}
			var c event.MessageChunk
			json.Unmarshal(ev.Data, &c)
			cur.WriteString(c.Content.Text)
			continue
		}
		if inReply && ev.Type != event.UsageUpdate {
			last.Reset()
			last.WriteString(cur.String())
			inReply = false
		}
	}
	if inReply {
		last.Reset()
		last.WriteString(cur.String())
	}
	return strings.TrimSpace(last.String())
}

// KeepCompare keeps one run of a comparison and archives the others.
func (e *Engine) KeepCompare(ctx context.Context, tag, keep string) error {
	runs, err := e.Compare(ctx, tag)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(runs, func(r CompareRun) bool { return r.Session.ID == keep }) {
		return errors.New("that session is not in the comparison")
	}
	for _, r := range runs {
		if r.Session.ID != keep && r.Session.Status != session.StatusArchived {
			if err := e.ArchiveSession(ctx, r.Session.ID, true); err != nil {
				return err
			}
		}
	}
	return nil
}
