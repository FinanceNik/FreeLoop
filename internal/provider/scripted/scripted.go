// Package scripted is a provider that plays a fixed script. Tests use it to
// drive the built-in harness without a network; development builds offer a
// demo script so the whole loop can be tried without an API key.
package scripted

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/FinanceNik/Freeloop/internal/provider"
)

// Call is one scripted tool call.
type Call struct {
	Name  string
	Input any
}

// NoSteps starts a prompt that gets a plain answer and no tool calls, so a check can
// leave a session with a large context and no further call to compact it.
const NoSteps = "[no steps]"

// PlanPrompt starts a prompt that gets a plan to approve (present_plan, which the
// built-in agent offers in plan mode), then a short answer once it is decided. Each
// plan of a conversation has one step more than the one before.
const PlanPrompt = "[plan]"

// AskPrompt starts a prompt that gets questions for the user (ask_user), then the
// answers said back once they come.
const AskPrompt = "[ask]"

// Step is one scripted model response.
type Step struct {
	Text  string
	Calls []Call
	// Stop overrides the stop reason; default tool_use with calls, else end_turn.
	Stop provider.StopReason
}

// Provider plays Steps. Which step answers a request follows from the
// request itself: the number of tool-result messages since the last user
// prompt. So one script serves any number of sessions and survives restarts.
type Provider struct {
	id    string
	steps []Step

	mu       sync.Mutex
	requests []provider.ChatRequest
}

// New returns a provider that answers with steps in order.
func New(id string, steps ...Step) *Provider {
	return &Provider{id: id, steps: steps}
}

func (p *Provider) ID() string          { return p.id }
func (p *Provider) Type() provider.Type { return provider.TypeScripted }

func (p *Provider) ListModels(context.Context) ([]provider.ModelInfo, error) {
	return []provider.ModelInfo{{ID: "scripted", DisplayName: "Scripted demo"}}, nil
}

func (p *Provider) Capabilities(string) provider.ModelCaps {
	return provider.ModelCaps{Source: provider.CapsStatic, Tools: true, ParallelTools: true, StableToolCallIDs: true, ContextWindow: 100_000, MaxOutput: 8_000}
}

// Requests returns every request received so far.
func (p *Provider) Requests() []provider.ChatRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.ChatRequest{}, p.requests...)
}

// position counts tool-result messages after the last prompt.
func position(msgs []provider.Message) int {
	n := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role != provider.RoleUser {
			continue
		}
		if len(m.Parts) > 0 && m.Parts[0].Type == provider.PartToolResult {
			n++
			continue
		}
		break
	}
	return n
}

func (p *Provider) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.Event, error) {
	p.mu.Lock()
	p.requests = append(p.requests, req)
	p.mu.Unlock()

	i := position(req.Messages)
	step := Step{Text: "(The script has no more steps.)"}
	if i < len(p.steps) {
		step = p.steps[i]
	}
	if asksNoSteps(req.Messages) {
		step = Step{Text: "Noted."}
	}
	if s, ok := planStep(req.Messages); ok {
		step = s
	}
	if s, ok := askStep(req.Messages); ok {
		step = s
	}
	if s, ok := workflowStep(req.Messages); ok {
		step = s
	}
	input := inputTokens(req.Messages)
	turn := len(p.Requests())

	ch := make(chan provider.Event, 64)
	go func() {
		defer close(ch)
		send := func(ev provider.Event) bool {
			select {
			case ch <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}
		msg := provider.Message{Role: provider.RoleAssistant}
		if step.Text != "" {
			for _, w := range strings.SplitAfter(step.Text, " ") {
				if !send(provider.Event{Type: provider.EventTextDelta, Text: w}) {
					return
				}
			}
			msg.Parts = append(msg.Parts, provider.Part{Type: provider.PartText, Text: step.Text})
		}
		for n, c := range step.Calls {
			id := fmt.Sprintf("call_%d_%d", turn, n)
			input, err := json.Marshal(c.Input)
			if err != nil {
				send(provider.Event{Type: provider.EventError, Err: err})
				return
			}
			if !send(provider.Event{Type: provider.EventToolCallStart, ToolCallID: id, ToolName: c.Name}) {
				return
			}
			msg.Parts = append(msg.Parts, provider.Part{Type: provider.PartToolUse, ToolCallID: id, ToolName: c.Name, Input: input})
		}
		stop := step.Stop
		if stop == "" {
			stop = provider.StopEndTurn
			if len(step.Calls) > 0 {
				stop = provider.StopToolUse
			}
		}
		if ctx.Err() != nil {
			send(provider.Event{Type: provider.EventError, Err: ctx.Err()})
			return
		}
		send(provider.Event{Type: provider.EventDone, Message: &msg, StopReason: stop,
			Usage: provider.Usage{InputTokens: input, OutputTokens: int64(len(step.Text)/4 + 10)}})
	}()
	return ch, nil
}

// askStep answers an AskPrompt: two questions while the prompt is the latest message,
// and the tool's result said back once it is answered.
func askStep(msgs []provider.Message) (Step, bool) {
	if len(msgs) == 0 {
		return Step{}, false
	}
	prompt := -1
	for i, m := range msgs {
		if m.Role == provider.RoleUser && len(m.Parts) > 0 && m.Parts[0].Type == provider.PartText {
			prompt = i
		}
	}
	if prompt < 0 || !strings.HasPrefix(msgs[prompt].Parts[0].Text, AskPrompt) {
		return Step{}, false
	}
	if prompt == len(msgs)-1 {
		questions := []map[string]any{
			{"question": "How should I estimate the tickets?", "header": "Order", "options": []map[string]any{
				{"label": "All at once", "description": "You paste all tickets first, then I estimate them together."},
				{"label": "One by one", "description": "I estimate each ticket as soon as you paste it."},
			}},
			{"question": "Which checks should run?", "header": "Checks", "multiSelect": true, "options": []map[string]any{
				{"label": "Tests"}, {"label": "Lint"}, {"label": "Types"},
			}},
		}
		return Step{Calls: []Call{{Name: "ask_user", Input: map[string]any{"questions": questions}}}}, true
	}
	last := msgs[len(msgs)-1]
	said := "nothing"
	for _, part := range last.Parts {
		if part.Type == provider.PartToolResult {
			said = part.Text
		}
	}
	return Step{Text: "Thanks. " + said}, true
}

// planStep answers a PlanPrompt: a plan while the prompt is the latest message, and a
// line of text once the plan has its answer.
func planStep(msgs []provider.Message) (Step, bool) {
	prompt := -1
	plans := 0
	for i, m := range msgs {
		if m.Role == provider.RoleUser && len(m.Parts) > 0 && m.Parts[0].Type == provider.PartText {
			prompt = i
		}
		for _, part := range m.Parts {
			if part.Type == provider.PartToolUse && part.ToolName == "present_plan" {
				plans++
			}
		}
	}
	if prompt < 0 || !strings.HasPrefix(msgs[prompt].Parts[0].Text, PlanPrompt) {
		return Step{}, false
	}
	if prompt != len(msgs)-1 {
		return Step{Text: "Understood."}, true
	}
	steps := []string{"Read the code", "Write the tests", "Change the code", "Ask before each change"}
	var b strings.Builder
	b.WriteString("# The plan\n\nWhat changes and why.\n\n")
	for i, s := range steps[:min(len(steps), plans+2)] {
		fmt.Fprintf(&b, "%d. %s\n", i+1, s)
	}
	b.WriteString("\n## Checks\n\nThe tests pass.")
	return Step{Calls: []Call{{Name: "present_plan", Input: map[string]any{"plan": b.String()}}}}, true
}

// lastPrompt is the text of the latest user prompt, not a tool's result, and how many
// tool results came after it.
func lastPrompt(msgs []provider.Message) (string, int) {
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role == provider.RoleUser && len(m.Parts) > 0 && m.Parts[0].Type == provider.PartText {
			return m.Parts[0].Text, position(msgs)
		}
	}
	return "", 0
}

var issueIn = regexp.MustCompile(`Your issue is (\S+?):`)

// workflowStep plays the built-in workflow's stages (WFL-002 to WFL-006), recognised by
// their prompts' first line, so the app check can run the workflow end to end: slicing
// writes two issues, the second blocked by the first, and commits them; an AFK session
// marks its issue done, writes a file and commits; a review finds nothing wrong.
func workflowStep(msgs []provider.Message) (Step, bool) {
	prompt, pos := lastPrompt(msgs)
	issue := func(id, title, blocked string) map[string]any {
		return map[string]any{"path": "backlog/issues/" + id + ".md", "content": fmt.Sprintf("---\ntitle: %s\nstatus: open\ntype: afk\nblockedBy: [%s]\nprd: demo\n---\n\n## What\n%s, end to end.\n\n## Acceptance\n- It works.\n", title, blocked, title)}
	}
	plays := map[string][]Step{
		"# Workflow stage: grill": {{Text: "First question: who is this for? I suggest: the people already using the app, who asked for it."}},
		"# Workflow stage: prd": {
			{Text: "I'll write the PRD.", Calls: []Call{{Name: "write_file", Input: map[string]any{"path": "backlog/prds/demo.md", "content": "---\ntitle: Demo\nstatus: open\n---\n\n## Problem\nA demo.\n"}}}},
			{Text: "The PRD is in backlog/prds/demo.md."},
		},
		"# Workflow stage: slice": {
			{Text: "Two vertical slices, the second after the first.", Calls: []Call{{Name: "write_file", Input: issue("001-first-slice", "First slice", "")}, {Name: "write_file", Input: issue("002-second-slice", "Second slice", "001-first-slice")}}},
			{Text: "Committing the backlog.", Calls: []Call{{Name: "shell", Input: map[string]any{"command": "git add backlog"}}}},
			{Calls: []Call{{Name: "shell", Input: map[string]any{"command": "git commit -q -m 'Slice the demo into issues'"}}}},
			{Text: "Sliced into backlog/issues/001-first-slice.md, then 002-second-slice.md, which waits on it. Both are AFK."},
		},
	}
	// A workflow's review of an issue (WFL-006), told what to look at by the workflow.
	if strings.HasPrefix(prompt, "Review the changes below") && strings.Contains(prompt, "whether the changes do what the issue") {
		return Step{Text: "Reviewed: the change does what the issue asks, and I found nothing wrong."}, true
	}
	for marker, steps := range plays {
		if strings.HasPrefix(prompt, marker) {
			if pos < len(steps) {
				return steps[pos], true
			}
			return Step{Text: "Done."}, true
		}
	}
	if !strings.HasPrefix(prompt, "# Workflow stage: afk") {
		return Step{}, false
	}
	m := issueIn.FindStringSubmatch(prompt)
	if m == nil {
		return Step{Text: "I cannot tell which issue is mine."}, true
	}
	path := m[1]
	id := strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".md")
	steps := []Step{
		{Text: "Test first, then the change; then I mark the issue done.", Calls: []Call{
			{Name: "write_file", Input: map[string]any{"path": "work/" + id + ".txt", "content": "Done by an AFK session: " + id + "\n"}},
			{Name: "edit_file", Input: map[string]any{"path": path, "old_string": "status: open", "new_string": "status: done"}},
		}},
		{Calls: []Call{{Name: "shell", Input: map[string]any{"command": "git add -A"}}}},
		{Calls: []Call{{Name: "shell", Input: map[string]any{"command": "git commit -q -m '" + id + ": done'"}}}},
		{Text: "Done: " + id + " is committed and marked done."},
	}
	if pos < len(steps) {
		return steps[pos], true
	}
	return Step{Text: "Done."}, true
}

// asksNoSteps reports whether the latest message starts with NoSteps.
func asksNoSteps(msgs []provider.Message) bool {
	if len(msgs) == 0 {
		return false
	}
	last := msgs[len(msgs)-1]
	return last.Role == provider.RoleUser && len(last.Parts) > 0 && strings.HasPrefix(last.Parts[0].Text, NoSteps)
}

// inputTokens is a small fixed count, or the size of what was sent once that is larger,
// so a long conversation fills the context as a real one does.
func inputTokens(msgs []provider.Message) int64 {
	var chars int
	for _, m := range msgs {
		for _, part := range m.Parts {
			chars += len(part.Text)
		}
	}
	return max(100, int64(chars/4))
}

// Demo is the development demo: look around, write a file (which asks for
// approval in ask mode), run a command, and summarize.
func Demo(id string) *Provider {
	return New(id,
		Step{Text: "I'll start by looking at the project.", Calls: []Call{{Name: "list_dir", Input: map[string]any{"path": "."}}}},
		Step{Text: "Now I'll write a small file.", Calls: []Call{{Name: "write_file", Input: map[string]any{
			"path": "freeloop-demo.txt", "content": "Written by the Freeloop scripted demo.\n",
		}}}},
		Step{Text: "And run a command to check it.", Calls: []Call{{Name: "shell", Input: map[string]any{"command": "ls -la freeloop-demo.txt && cat freeloop-demo.txt"}}}},
		Step{Text: "Done. This was the scripted demo provider: it listed the project, wrote `freeloop-demo.txt` and ran `ls`. Add an Anthropic provider to work with a real model."},
	)
}

// Tour is the in-app demo (GUI-045): one turn that meets each answer the permission
// policy gives, in order: allowed without asking, asks, refused by a rule, refused as
// a protected path, and asks because a file's name says it holds secrets. It needs
// the session to carry the rule shell(git push *), which the engine adds.
func Tour(id string) *Provider {
	return New(id,
		Step{Text: "This is a demo of what Freeloop does when an agent acts. Nothing here touches your own files; the project is a scratch folder. First I will read README.md. Reading a file inside the project is allowed without asking.",
			Calls: []Call{{Name: "read_file", Input: map[string]any{"path": "README.md"}}}},
		Step{Text: "That went ahead without a prompt. Next I will write notes.md. Changing a file asks you first, because this session is in ask mode. Allow or deny it below.",
			Calls: []Call{{Name: "write_file", Input: map[string]any{"path": "notes.md", "content": "Notes from the Freeloop demo.\n"}}}},
		Step{Text: "Now I will run `git push origin main`. This session has a rule that denies `shell(git push *)`. A deny rule holds in every mode, bypass included, so you are not asked and the command does not run.",
			Calls: []Call{{Name: "shell", Input: map[string]any{"command": "git push origin main"}}}},
		Step{Text: "Next I will write a git hook, .git/hooks/pre-commit. Hooks run code on every commit, so Freeloop protects the .git folder. No mode or rule can unlock a protected path.",
			Calls: []Call{{Name: "write_file", Input: map[string]any{"path": ".git/hooks/pre-commit", "content": "#!/bin/sh\necho hooked\n"}}}},
		Step{Text: "Last I will read .env. The file's name says it holds secrets, so Freeloop asks even though it is inside the project. Here the value is fake.",
			Calls: []Call{{Name: "read_file", Input: map[string]any{"path": ".env"}}}},
		Step{Text: "That is the tour. Allowed: reading README.md. Asked: writing notes.md and reading .env. Refused: git push, by a rule, and the hook, as a protected path. To see why for any call, open Settings, Policy and use \"Would this be allowed?\". To see what was sent to the model, open this session's Info tab and \"What the model sees\". \"Remove demo\" deletes the session and the scratch project."},
	)
}

// Steps returns the script.
func (p *Provider) Steps() []Step { return append([]Step{}, p.steps...) }
