// Package web turns fetched pages into text and reads search results for
// the built-in harness's web tools (HRN-017). It does no networking.
package web

import (
	"encoding/json"
	"errors"
	"html"
	"regexp"
	"strings"
)

var (
	hidden     = regexp.MustCompile(`(?is)<(script|style|noscript|svg|template|head|title)\b.*?</(script|style|noscript|svg|template|head|title)\s*>|<!--.*?-->`)
	titleTag   = regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title\s*>`)
	blockTag   = regexp.MustCompile(`(?i)</?(p|div|section|article|header|footer|main|nav|aside|ul|ol|li|tr|table|h[1-6]|pre|blockquote|hr|br)\b[^>]*>`)
	anyTag     = regexp.MustCompile(`(?s)<[^>]*>`)
	blanks     = regexp.MustCompile(`[ \t\r\f\v]+`)
	blankLines = regexp.MustCompile(`\n\s*\n\s*\n+`)
)

// Text returns a page's title and readable text: scripts, styles and
// markup are gone, block elements end a line. It reads what is there and
// runs nothing.
func Text(page string) (title, text string) {
	if m := titleTag.FindStringSubmatch(page); m != nil {
		title = strings.TrimSpace(html.UnescapeString(anyTag.ReplaceAllString(m[1], "")))
	}
	s := hidden.ReplaceAllString(page, "")
	s = blockTag.ReplaceAllString(s, "\n")
	s = anyTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(blanks.ReplaceAllString(l, " "))
	}
	s = blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return title, strings.TrimSpace(s)
}

// Result is one search hit.
type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// ParseResults reads a search endpoint's JSON: SearXNG's
// results[].{title,url,content} or Brave's web.results[].{title,url,description}.
func ParseResults(b []byte) ([]Result, error) {
	var d struct {
		Results []struct {
			Title, URL, Content, Description string
		} `json:"results"`
		Web struct {
			Results []struct {
				Title, URL, Content, Description string
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, errors.New("the search endpoint did not answer with JSON")
	}
	raw := d.Results
	if len(raw) == 0 {
		raw = d.Web.Results
	}
	out := []Result{}
	for _, r := range raw {
		if r.URL == "" {
			continue
		}
		snippet := r.Content
		if snippet == "" {
			snippet = r.Description
		}
		_, snippet = Text(snippet)
		out = append(out, Result{Title: strings.TrimSpace(r.Title), URL: r.URL, Snippet: snippet})
	}
	return out, nil
}
