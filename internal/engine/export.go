package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// Export (SES-008): a session as Markdown to read or share, or as JSON with
// every event, to keep or process. Both come from the transcript, so an
// export shows what the conversation view shows.

// Export formats.
const (
	ExportMarkdown = "markdown"
	ExportJSON     = "json"
)

// exportOutputCap bounds a tool's output in a Markdown export.
const exportOutputCap = 8 << 10

// RenderExport returns a session in format.
func (e *Engine) RenderExport(ctx context.Context, sid, format string) (string, error) {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return "", err
	}
	events, err := e.store.Events(ctx, sid, 0)
	if err != nil {
		return "", err
	}
	switch format {
	case ExportJSON:
		b, err := json.MarshalIndent(struct {
			Session  session.Session `json:"session"`
			Exported time.Time       `json:"exported"`
			Events   []event.Event   `json:"events"`
		}{ses, time.Now().UTC(), events}, "", "  ")
		return string(b) + "\n", err
	case ExportMarkdown:
		return renderMarkdown(ses, events), nil
	}
	return "", fmt.Errorf("unknown export format %q", format)
}

// ExportSession writes a session in format to path, through a local
// environment like every file the app writes.
func (e *Engine) ExportSession(ctx context.Context, sid, format, path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("the export needs an absolute path")
	}
	text, err := e.RenderExport(ctx, sid, format)
	if err != nil {
		return err
	}
	env, err := e.localEnv(filepath.Dir(path), nil)
	if err != nil {
		return err
	}
	return env.FS().WriteFile(path, []byte(text))
}

// ExportFileName suggests a file name for a session's export.
func ExportFileName(title, format string) string {
	ext := ".md"
	if format == ExportJSON {
		ext = ".json"
	}
	name := slug(title)
	if name == "" {
		name = "session"
	}
	return name + ext
}

// mdBlock is a part of the Markdown export; tool calls are filled in as
// their updates arrive, in the order the calls were made.
type mdBlock struct {
	text string
	tool *mdTool
}

type mdTool struct {
	title, kind, input, status, decision string
	output                               []string
	diffs                                []string
}

func renderMarkdown(ses session.Session, events []event.Event) string {
	var blocks []mdBlock
	tools := map[string]*mdTool{}
	// Streamed text collects until the kind (agent or thought) changes.
	var text strings.Builder
	var kind event.Type
	flush := func() {
		s := strings.TrimSpace(text.String())
		switch {
		case s == "":
		case kind == event.AgentThoughtChunk:
			blocks = append(blocks, mdBlock{text: "<details><summary>Thinking</summary>\n\n" + s + "\n\n</details>"})
		default:
			blocks = append(blocks, mdBlock{text: "**Agent**\n\n" + s})
		}
		text.Reset()
	}
	note := func(s string) {
		flush()
		blocks = append(blocks, mdBlock{text: "> " + strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n> ")})
	}
	for _, ev := range events {
		var d map[string]any
		json.Unmarshal(ev.Data, &d)
		str := func(k string) string { s, _ := d[k].(string); return s }
		switch ev.Type {
		case event.UserMessageChunk:
			flush()
			var c event.MessageChunk
			json.Unmarshal(ev.Data, &c)
			if c.Content.Type == "resource_link" {
				blocks = append(blocks, mdBlock{text: "*Attached: " + cmpOr(c.Content.Name, c.Content.URI) + "*"})
				continue
			}
			if c.Content.Type == "image" {
				// The picture stays in the session's attachments folder.
				blocks = append(blocks, mdBlock{text: "*Attached picture: " + strings.TrimPrefix(c.Content.URI, "attachment:") + "*"})
				continue
			}
			blocks = append(blocks, mdBlock{text: "## You\n\n" + strings.TrimSpace(c.Content.Text)})
		case event.AgentMessageChunk, event.AgentThoughtChunk:
			var c event.MessageChunk
			json.Unmarshal(ev.Data, &c)
			if ev.Type != kind {
				flush()
				kind = ev.Type
			}
			text.WriteString(c.Content.Text)
		case event.ToolCall:
			flush()
			t := &mdTool{title: cmpOr(str("title"), str("tool"), "Tool call"), kind: str("kind"), status: str("status")}
			if in, ok := d["rawInput"]; ok && in != nil {
				b, _ := json.MarshalIndent(in, "", "  ")
				t.input = string(b)
			}
			tools[str("toolCallId")] = t
			blocks = append(blocks, mdBlock{tool: t})
		case event.ToolCallUpdate:
			t := tools[str("toolCallId")]
			if t == nil {
				continue
			}
			if s := str("status"); s != "" {
				t.status = s
			}
			var u struct {
				Content []struct {
					Type    string             `json:"type"`
					Content event.ContentBlock `json:"content"`
					Path    string             `json:"path"`
					OldText *string            `json:"oldText"`
					NewText string             `json:"newText"`
				} `json:"content"`
			}
			json.Unmarshal(ev.Data, &u)
			for _, c := range u.Content {
				switch c.Type {
				case "content":
					if c.Content.Text != "" {
						t.output = append(t.output, c.Content.Text)
					}
				case "diff":
					old := ""
					if c.OldText != nil {
						old = *c.OldText
					}
					t.diffs = append(t.diffs, renderDiff(c.Path, old, c.NewText))
				}
			}
		case event.PermissionDecided:
			if t := tools[str("toolCallId")]; t != nil {
				t.decision = fmt.Sprintf("%s by %s", str("outcome"), str("actor"))
				if r := str("reason"); r != "" {
					t.decision += ": " + r
				}
			}
		case event.TurnEnded:
			flush()
			stop := str("stopReason")
			if stop != "" && stop != "end_turn" {
				blocks = append(blocks, mdBlock{text: "*Turn ended: " + stop + "*"})
			}
			blocks = append(blocks, mdBlock{text: "---"})
		case event.CompactionUpdate:
			if str("status") == "completed" {
				note("Context compacted (" + str("reason") + ").")
			}
		case event.ModelChanged:
			note("Model changed to " + str("model") + ".")
		case event.Rewind:
			note("Rewound to an earlier checkpoint; the turns after it were undone.")
		case event.PullRequest:
			var pr event.PullRequestData
			json.Unmarshal(ev.Data, &pr)
			note(fmt.Sprintf("Opened pull request [%s](%s) (%s into %s).", strings.ReplaceAll(pr.Title, "]", "\\]"), pr.URL, pr.Head, pr.Base))
		case event.Error:
			note("**Error:** " + str("message"))
		case event.Notice:
			note(str("message"))
		case event.HookRan:
			var h HookRanData
			json.Unmarshal(ev.Data, &h)
			if h.Blocked {
				note(fmt.Sprintf("A %s hook blocked it: %s", h.Event, cmpOr(strings.TrimSpace(h.Stderr), h.Command)))
			}
		}
	}
	flush()

	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", ses.Title)
	fmt.Fprintf(&sb, "- Project: `%s`\n", ses.ProjectRoot)
	if ses.Model != "" {
		fmt.Fprintf(&sb, "- Model: %s · %s\n", ses.ProviderID, ses.Model)
	} else {
		fmt.Fprintf(&sb, "- Harness: %s\n", ses.HarnessID)
	}
	fmt.Fprintf(&sb, "- Started: %s\n", ses.CreatedAt.Local().Format("2006-01-02 15:04"))
	fmt.Fprintf(&sb, "- Exported: %s\n", time.Now().Local().Format("2006-01-02 15:04"))
	for _, b := range blocks {
		sb.WriteString("\n")
		if b.tool == nil {
			sb.WriteString(b.text)
			sb.WriteString("\n")
			continue
		}
		t := b.tool
		fmt.Fprintf(&sb, "### %s\n\n", strings.ReplaceAll(t.title, "\n", " "))
		var meta []string
		if t.kind != "" {
			meta = append(meta, t.kind)
		}
		if t.status != "" {
			meta = append(meta, t.status)
		}
		if t.decision != "" {
			meta = append(meta, t.decision)
		}
		if len(meta) > 0 {
			fmt.Fprintf(&sb, "*%s*\n\n", strings.Join(meta, " · "))
		}
		if t.input != "" && t.input != "null" && t.input != "{}" {
			sb.WriteString(fence("json", t.input))
		}
		for _, df := range t.diffs {
			sb.WriteString(fence("diff", df))
		}
		if out := strings.Join(t.output, "\n"); strings.TrimSpace(out) != "" {
			if len(out) > exportOutputCap {
				out = out[:exportOutputCap] + fmt.Sprintf("\n[%d more bytes left out]", len(out)-exportOutputCap)
			}
			sb.WriteString(fence("", out))
		}
	}
	return sb.String()
}

func cmpOr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// fence wraps s in a code fence longer than any backtick run inside it.
func fence(lang, s string) string {
	ticks := "```"
	for strings.Contains(s, ticks) {
		ticks += "`"
	}
	return ticks + lang + "\n" + strings.TrimRight(s, "\n") + "\n" + ticks + "\n\n"
}

// renderDiff is a unified-style diff of an edit: the changed lines between
// the common start and end, with up to two lines of context.
func renderDiff(path, old, new string) string {
	a := strings.Split(strings.TrimSuffix(old, "\n"), "\n")
	b := strings.Split(strings.TrimSuffix(new, "\n"), "\n")
	if old == "" {
		a = nil
	}
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", path, path)
	for _, l := range a[max(0, pre-2):pre] {
		sb.WriteString(" " + l + "\n")
	}
	for _, l := range a[pre : len(a)-suf] {
		sb.WriteString("-" + l + "\n")
	}
	for _, l := range b[pre : len(b)-suf] {
		sb.WriteString("+" + l + "\n")
	}
	for _, l := range a[len(a)-suf : min(len(a), len(a)-suf+2)] {
		sb.WriteString(" " + l + "\n")
	}
	return sb.String()
}
