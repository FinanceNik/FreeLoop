package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
)

// Limits for files embedded from @ mentions (GUI-004).
const (
	maxMentions      = 20
	maxMentionBytes  = 256 << 10
	maxMentionTotal  = 1 << 20
	maxProjectFiles  = 20000
	mentionTrailers  = ".,;:!?)]}'"
	mentionNotFound  = "not found"
	mentionTooLarge  = "larger than 256 KB"
	mentionOverTotal = "over the 1 MB limit for one message"
)

// An @ starts a mention at the start of the text or after whitespace, so
// e-mail addresses are not mentions. Paths with spaces are quoted.
var mentionPattern = regexp.MustCompile(`(?:^|\s)@(?:"([^"\n]+)"|(\S+))`)

// mentionPaths returns the distinct paths text mentions, in order.
func mentionPaths(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range mentionPattern.FindAllStringSubmatch(text, -1) {
		p := m[1]
		if p == "" {
			p = strings.TrimRight(m[2], mentionTrailers)
		}
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// embedMentions reads the files a prompt mentions. It reads through the
// environment, so protected paths stay refused, and only inside the project
// root: a mention is the user's own choice of context, like pasting the
// file, not a tool call. Mentions that name no file stay plain text; files
// that exist but cannot be attached are reported in a notice.
func (e *Engine) embedMentions(sid string, env environment.Environment, text string) harness.Prompt {
	p := harness.Prompt{Text: text}
	var skipped []string
	total := 0
	for _, m := range mentionPaths(text) {
		if len(p.Resources) == maxMentions {
			break
		}
		r, err := readMention(env, m)
		if errors.Is(err, errNotMentionable) {
			continue
		}
		if err == nil && total+len(r.Text) > maxMentionTotal {
			err = errors.New(mentionOverTotal)
		}
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("@%s (%v)", m, err))
			continue
		}
		total += len(r.Text)
		p.Resources = append(p.Resources, r)
	}
	if len(skipped) > 0 {
		e.emit(sid, event.Notice, map[string]string{"message": "Not attached: " + strings.Join(skipped, ", ")})
	}
	return p
}

// errNotMentionable marks a mention that names no file in the project.
var errNotMentionable = errors.New(mentionNotFound)

func readMention(env environment.Environment, rel string) (harness.Resource, error) {
	abs := env.Resolve(rel)
	if !env.Contains(abs) {
		return harness.Resource{}, errNotMentionable
	}
	fi, err := env.FS().Stat(abs)
	if err != nil {
		if errors.Is(err, environment.ErrProtected) {
			return harness.Resource{}, errors.New("protected path")
		}
		return harness.Resource{}, errNotMentionable
	}
	if fi.IsDir {
		return harness.Resource{}, errNotMentionable
	}
	if fi.Size > maxMentionBytes {
		return harness.Resource{}, errors.New(mentionTooLarge)
	}
	b, err := env.FS().ReadFile(abs)
	if errors.Is(err, environment.ErrProtected) {
		return harness.Resource{}, errors.New("protected path")
	}
	if err != nil {
		return harness.Resource{}, err
	}
	if len(b) > maxMentionBytes {
		return harness.Resource{}, errors.New(mentionTooLarge)
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return harness.Resource{}, errors.New("binary file")
	}
	return harness.Resource{
		Path: filepath.ToSlash(fi.Path),
		URI:  (&url.URL{Scheme: "file", Path: abs}).String(),
		Text: string(b),
	}, nil
}

// ProjectFiles lists the files of a session's project for the @ picker,
// relative to the root and without .git and node_modules (GUI-004).
func (e *Engine) ProjectFiles(ctx context.Context, sid string) ([]string, error) {
	e.mu.Lock()
	r, ok := e.runtimes[sid]
	e.mu.Unlock()
	var env environment.Environment
	if ok {
		env = r.env
	} else {
		ses, err := e.store.GetSession(ctx, sid)
		if err != nil {
			return nil, err
		}
		if !environment.IsDir(ses.ProjectRoot) {
			return nil, fmt.Errorf("the project folder %s is missing; relink the project", ses.ProjectRoot)
		}
		if env, err = e.localEnv(ses.ProjectRoot, nil); err != nil {
			return nil, err
		}
	}
	files, err := env.FS().Glob(ctx, "", "**", maxProjectFiles)
	if files == nil {
		files = []string{}
	}
	return files, err
}
