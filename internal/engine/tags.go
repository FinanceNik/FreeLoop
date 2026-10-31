package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// Limits on a session's tags (SES-013).
const (
	maxTags   = 16
	maxTagLen = 32
)

// SetTags replaces a session's tags (SES-013). Each is cleaned the way
// NormalizeTags says; the change is an event, so a fork keeps the tags and
// the sidebar follows them live.
func (e *Engine) SetTags(ctx context.Context, sid string, tags []string) ([]string, error) {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	clean := NormalizeTags(tags)
	if len(clean) > maxTags {
		return nil, fmt.Errorf("a session takes at most %d tags", maxTags)
	}
	if slices.Equal(clean, ses.Tags) {
		return clean, nil
	}
	e.emit(sid, event.TagsChanged, event.TagsData{Tags: clean})
	return clean, nil
}

// NormalizeTags lower-cases tags, drops a leading #, joins words with
// dashes, cuts each to 32 characters, and returns them sorted without
// empty ones or repeats.
func NormalizeTags(tags []string) []string {
	out := []string{}
	for _, t := range tags {
		t = strings.ToLower(strings.TrimLeft(strings.TrimSpace(t), "#"))
		t = strings.Join(strings.FieldsFunc(t, unicode.IsSpace), "-")
		if r := []rune(t); len(r) > maxTagLen {
			t = string(r[:maxTagLen])
		}
		if t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	slices.Sort(out)
	return out
}
