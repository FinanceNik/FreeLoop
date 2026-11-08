package engine

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// Starting from a GitHub issue (SES-017). The new-session form lists a
// trusted project's open issues through the GitHub CLI and turns one into
// the session's first message, which the user reads and edits before
// sending: an issue is text anyone can write, so it is never sent on its
// own.

// Issue is one open issue of a project's GitHub repository.
type Issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	Labels    []string  `json:"labels"`
	Author    string    `json:"author"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// IssueBrief is an issue written as a first message.
type IssueBrief struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	// SessionTitle is "#12 Title"; Tag is "issue-12".
	SessionTitle string `json:"sessionTitle"`
	Tag          string `json:"tag"`
	Text         string `json:"text"`
}

// maxBrief caps an issue's message.
const maxBrief = 20 << 10

// issueEnv returns where a project's gh commands run: its main checkout,
// once trusted.
func (e *Engine) issueEnv(ctx context.Context, root string) (environment.Environment, error) {
	if netguard.LocalOnly() {
		return nil, errors.New("the app is set to connect to nothing but this computer")
	}
	if root == "" || !environment.IsDir(root) {
		return nil, errors.New("choose a project folder first")
	}
	p, err := e.store.ProjectByRoot(ctx, environment.Canonical(root))
	if err != nil || !p.Trusted {
		return nil, errors.New("trust the project to list its GitHub issues")
	}
	return e.localEnv(p.Root, nil)
}

type ghIssue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	Body      string    `json:"body"`
	UpdatedAt time.Time `json:"updatedAt"`
	Author    struct {
		Login string `json:"login"`
	} `json:"author"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Comments []struct {
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		Body string `json:"body"`
	} `json:"comments"`
}

func ghFailed(what string, res environment.ExecResult) error {
	msg := strings.TrimSpace(res.Stderr)
	if msg == "" {
		msg = strings.TrimSpace(res.Output)
	}
	return fmt.Errorf("%s failed: %s", what, msg)
}

// Issues lists a project's open GitHub issues, the 50 most recently
// updated, matching search when it is set (GitHub's issue search syntax).
func (e *Engine) Issues(ctx context.Context, root, search string) ([]Issue, error) {
	env, err := e.issueEnv(ctx, root)
	if err != nil {
		return nil, err
	}
	args := []string{"issue", "list", "--state", "open", "--limit", "50", "--json", "number,title,url,labels,author,updatedAt"}
	if s := strings.TrimSpace(search); s != "" {
		args = append(args, "--search", s)
	}
	res, err := e.runGH(ctx, env, args...)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, ghFailed("gh issue list", res)
	}
	var raw []ghIssue
	if err := json.Unmarshal([]byte(res.Output), &raw); err != nil {
		return nil, fmt.Errorf("gh issue list printed something else: %.200s", res.Output)
	}
	out := make([]Issue, 0, len(raw))
	for _, r := range raw {
		is := Issue{Number: r.Number, Title: r.Title, URL: r.URL, Author: r.Author.Login, UpdatedAt: r.UpdatedAt, Labels: []string{}}
		for _, l := range r.Labels {
			is.Labels = append(is.Labels, l.Name)
		}
		out = append(out, is)
	}
	return out, nil
}

// IssueBrief reads one issue with its comments and writes it as a first
// message.
func (e *Engine) IssueBrief(ctx context.Context, root string, number int) (IssueBrief, error) {
	if number <= 0 {
		return IssueBrief{}, errors.New("not an issue number")
	}
	env, err := e.issueEnv(ctx, root)
	if err != nil {
		return IssueBrief{}, err
	}
	res, err := e.runGH(ctx, env, "issue", "view", strconv.Itoa(number), "--json", "number,title,url,body,labels,author,comments")
	if err != nil {
		return IssueBrief{}, err
	}
	if res.ExitCode != 0 {
		return IssueBrief{}, ghFailed("gh issue view", res)
	}
	var is ghIssue
	if err := json.Unmarshal([]byte(res.Output), &is); err != nil {
		return IssueBrief{}, fmt.Errorf("gh issue view printed something else: %.200s", res.Output)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Work on GitHub issue #%d: %s\n%s\n", is.Number, is.Title, is.URL)
	if len(is.Labels) > 0 {
		var names []string
		for _, l := range is.Labels {
			names = append(names, l.Name)
		}
		fmt.Fprintf(&b, "Labels: %s\n", strings.Join(names, ", "))
	}
	if body := strings.TrimSpace(is.Body); body != "" {
		fmt.Fprintf(&b, "\n%s\n", body)
	}
	if len(is.Comments) > 0 {
		b.WriteString("\n## Comments\n")
		for _, c := range is.Comments {
			fmt.Fprintf(&b, "\n%s wrote:\n%s\n", cmp.Or(c.Author.Login, "someone"), strings.TrimSpace(c.Body))
		}
	}
	text := b.String()
	if len(text) > maxBrief {
		text = cutBytes(text, maxBrief) + fmt.Sprintf("\n\n[The issue is longer; read the rest at %s]\n", is.URL)
	}
	return IssueBrief{
		Number: is.Number, Title: is.Title, Text: text,
		SessionTitle: fmt.Sprintf("#%d %s", is.Number, is.Title), Tag: fmt.Sprintf("issue-%d", is.Number),
	}, nil
}
