package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
)

// maxToolOutput caps what one tool result sends to the model.
const maxToolOutput = 30_000

// tool is one built-in tool.
type tool struct {
	spec     provider.ToolSpec
	kind     permission.Kind
	readOnly bool // offered in plan mode
	// prepare validates the input and describes the call for the broker.
	prepare func(s *session, input json.RawMessage) (*call, error)
}

// call is a validated tool call, ready to be approved and run.
type call struct {
	title   string
	paths   []string
	command string
	options []permission.Option
	// permTool names the call for the broker when it differs from the
	// tool, as skill:<name> for load_skill (SEC-002).
	permTool string
	// contained and leaves are the shell's standing with the sandbox (ENV-009).
	contained permission.Containment
	leaves    bool
	// form asks the user for input instead of approval (HRN-021).
	form *permission.Form
	run  func(ctx context.Context, d permission.Decision) (result, error)
}

// result is what a tool returns: text for the model and display content for
// the transcript.
type result struct {
	text    string
	content []any
}

func textContent(s string) map[string]any {
	return map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": s}}
}

// A diff goes into the transcript, and every later read of the session loads it,
// so a small edit of a large file must not store the file twice on every edit.
const (
	wholeDiffUpTo  = 64 << 10
	storedDiffUpTo = 256 << 10
	diffContext    = 3
)

// shortDiff drops what both sides share, apart from a few lines around the change,
// and names a change that is still too large rather than storing it.
func shortDiff(oldText, newText string) (string, string) {
	if len(oldText)+len(newText) <= wholeDiffUpTo {
		return oldText, newText
	}
	a, b := strings.SplitAfter(oldText, "\n"), strings.SplitAfter(newText, "\n")
	head := 0
	for head < len(a) && head < len(b) && a[head] == b[head] {
		head++
	}
	tail := 0
	for tail < len(a)-head && tail < len(b)-head && a[len(a)-1-tail] == b[len(b)-1-tail] {
		tail++
	}
	head, tail = max(0, head-diffContext), max(0, tail-diffContext)
	oldText = strings.Join(a[head:len(a)-tail], "")
	newText = strings.Join(b[head:len(b)-tail], "")
	if len(oldText)+len(newText) > storedDiffUpTo {
		return "", fmt.Sprintf("(A change of %d bytes replaced by %d, too large to show here. The file holds it.)\n", len(oldText), len(newText))
	}
	return oldText, newText
}

func diffContent(path, oldText, newText string) map[string]any {
	oldText, newText = shortDiff(oldText, newText)
	return map[string]any{"type": "diff", "path": path, "oldText": oldText, "newText": newText}
}

func schema(required []string, props map[string]any) map[string]any {
	// Never null: Azure and OpenAI refuse the whole request over it, where Anthropic lets it pass.
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func str(desc string) map[string]any  { return map[string]any{"type": "string", "description": desc} }
func num(desc string) map[string]any  { return map[string]any{"type": "integer", "description": desc} }
func flag(desc string) map[string]any { return map[string]any{"type": "boolean", "description": desc} }

// decode unmarshals input and checks that every required string is set.
func decode(input json.RawMessage, v any, required ...string) error {
	if len(input) == 0 || !json.Valid(input) {
		return errors.New("the tool input is not valid JSON (it may have been cut off)")
	}
	if err := json.Unmarshal(input, v); err != nil {
		return fmt.Errorf("invalid input: %v", err)
	}
	var m map[string]any
	json.Unmarshal(input, &m)
	for _, r := range required {
		if s, ok := m[r].(string); !ok || s == "" {
			return fmt.Errorf("missing required field %q", r)
		}
	}
	return nil
}

func tools() []*tool {
	return []*tool{readFile(), listDir(), globTool(), grepTool(), writeFile(), editFile(), shellTool(), presentPlan(), askUser(), loadSkill(), taskTool(), webFetch(), webSearch(), todoWrite()}
}

func readFile() *tool {
	return &tool{
		spec: provider.ToolSpec{
			Name:        "read_file",
			Description: "Read a text file. Returns lines prefixed with their line numbers. Use offset and limit for large files.",
			InputSchema: schema([]string{"path"}, map[string]any{
				"path":   str("File path, relative to the project root or absolute."),
				"offset": num("First line to return, 1-based. Default 1."),
				"limit":  num("Maximum number of lines. Default 2000."),
			}),
		},
		kind: permission.KindRead, readOnly: true,
		prepare: func(s *session, input json.RawMessage) (*call, error) {
			var in struct {
				Path          string
				Offset, Limit int
			}
			if err := decode(input, &in, "path"); err != nil {
				return nil, err
			}
			abs := s.env.Resolve(in.Path)
			return &call{title: "Read " + in.Path, paths: []string{abs}, run: func(context.Context, permission.Decision) (result, error) {
				b, err := s.env.FS().ReadFile(abs)
				if err != nil {
					return result{}, err
				}
				lines := strings.Split(string(b), "\n")
				start := max(in.Offset, 1)
				limit := in.Limit
				if limit <= 0 {
					limit = 2000
				}
				var sb strings.Builder
				for i := start - 1; i < len(lines) && i < start-1+limit; i++ {
					line := lines[i]
					if len(line) > 2000 {
						line = line[:2000] + "…"
					}
					fmt.Fprintf(&sb, "%6d\t%s\n", i+1, line)
				}
				if sb.Len() == 0 {
					return result{text: "(empty file or offset past the end)"}, nil
				}
				return result{text: sb.String()}, nil
			}}, nil
		},
	}
}

func listDir() *tool {
	return &tool{
		spec: provider.ToolSpec{
			Name:        "list_dir",
			Description: "List a directory. Directories end in /.",
			InputSchema: schema(nil, map[string]any{"path": str("Directory, relative to the project root. Default the root.")}),
		},
		kind: permission.KindRead, readOnly: true,
		prepare: func(s *session, input json.RawMessage) (*call, error) {
			var in struct{ Path string }
			if err := decode(input, &in); err != nil {
				return nil, err
			}
			if in.Path == "" {
				in.Path = "."
			}
			abs := s.env.Resolve(in.Path)
			return &call{title: "List " + in.Path, paths: []string{abs}, run: func(context.Context, permission.Decision) (result, error) {
				entries, err := s.env.FS().ReadDir(abs)
				if err != nil {
					return result{}, err
				}
				var sb strings.Builder
				for _, e := range entries {
					sb.WriteString(e.Name)
					if e.IsDir {
						sb.WriteString("/")
					}
					sb.WriteString("\n")
				}
				return result{text: sb.String()}, nil
			}}, nil
		},
	}
}

func globTool() *tool {
	return &tool{
		spec: provider.ToolSpec{
			Name:        "glob",
			Description: "Find files by name pattern; ** matches any number of directories. Skips .git and node_modules.",
			InputSchema: schema([]string{"pattern"}, map[string]any{
				"pattern": str("Pattern such as **/*.go or src/**/test_*.py."),
				"path":    str("Directory to search, relative to the project root. Default the root."),
			}),
		},
		kind: permission.KindSearch, readOnly: true,
		prepare: func(s *session, input json.RawMessage) (*call, error) {
			var in struct{ Pattern, Path string }
			if err := decode(input, &in, "pattern"); err != nil {
				return nil, err
			}
			if in.Path == "" {
				in.Path = "."
			}
			abs := s.env.Resolve(in.Path)
			return &call{title: "Find " + in.Pattern, paths: []string{abs}, run: func(ctx context.Context, _ permission.Decision) (result, error) {
				paths, err := s.env.FS().Glob(ctx, abs, in.Pattern, 200)
				if err != nil {
					return result{}, err
				}
				paths = slices.DeleteFunc(paths, func(p string) bool { return s.hidden(p) })
				if len(paths) == 0 {
					return result{text: "No files matched."}, nil
				}
				return result{text: strings.Join(paths, "\n")}, nil
			}}, nil
		},
	}
}

// hidden reports whether a search result is a file a deny rule refuses to
// read, so a search over its folder does not show it either.
func (s *session) hidden(p string) bool {
	return s.p.Broker.Hidden(s.p.SessionID, s.env.Root(), s.env.Resolve(p))
}

func grepTool() *tool {
	return &tool{
		spec: provider.ToolSpec{
			Name:        "grep",
			Description: "Search file contents with a regular expression (Go RE2 syntax). Returns path:line: text. Skips .git, node_modules and binary files.",
			InputSchema: schema([]string{"pattern"}, map[string]any{
				"pattern":     str("Regular expression."),
				"path":        str("Directory to search, relative to the project root. Default the root."),
				"include":     str("Only files matching this glob, such as *.go."),
				"ignore_case": flag("Case-insensitive match."),
			}),
		},
		kind: permission.KindSearch, readOnly: true,
		prepare: func(s *session, input json.RawMessage) (*call, error) {
			var in struct {
				Pattern, Path, Include string
				IgnoreCase             bool `json:"ignore_case"`
			}
			if err := decode(input, &in, "pattern"); err != nil {
				return nil, err
			}
			if in.Path == "" {
				in.Path = "."
			}
			abs := s.env.Resolve(in.Path)
			paths := []string{abs}
			// An include that names secret files asks for them, so the user is asked as
			// for a read; without one a search leaves such files out (SEC-017).
			named := s.namesSecret(abs)
			if v := secretInclude(in.Include); v != "" {
				paths = append(paths, filepath.Join(abs, filepath.FromSlash(v)))
				named = true
			}
			return &call{title: "Search for " + in.Pattern, paths: paths, run: func(ctx context.Context, d permission.Decision) (result, error) {
				var left []string
				var opts []environment.GrepOption
				// A rule that allows every search and bypass mode are the user's choice of
				// no questions; anything else only decided about what was named.
				if !named && d.Actor != permission.ActorRule && d.Actor != permission.ActorMode {
					opts = append(opts, environment.SkipSecrets(&left))
				}
				matches, err := s.env.FS().Grep(ctx, abs, in.Pattern, in.Include, in.IgnoreCase, 200, opts...)
				if err != nil {
					return result{}, err
				}
				matches = slices.DeleteFunc(matches, func(m environment.Match) bool { return s.hidden(m.Path) })
				var sb strings.Builder
				for _, m := range matches {
					fmt.Fprintf(&sb, "%s:%d: %s\n", m.Path, m.Line, m.Text)
				}
				if sb.Len() == 0 {
					sb.WriteString("No matches.\n")
				}
				if len(left) > 0 {
					shown := left[:min(len(left), 10)]
					more := ""
					if len(left) > len(shown) {
						more = fmt.Sprintf(" and %d more", len(left)-len(shown))
					}
					fmt.Fprintf(&sb, "%d file(s) with a secret's name were left out: %s%s. Search one by naming it as the path or the include; the user is asked first.\n", len(left), strings.Join(shown, ", "), more)
				}
				return result{text: sb.String()}, nil
			}}, nil
		},
	}
}

// namesSecret reports a path inside the project whose name says it holds secrets.
func (s *session) namesSecret(abs string) bool {
	rel, err := filepath.Rel(s.env.Root(), abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return environment.SecretPath(filepath.ToSlash(rel))
}

// secretInclude returns a path an include glob stands for when it would match files named
// like secrets (".env", "*.pem", "**/secrets.yaml"), or "" when it would not.
func secretInclude(include string) string {
	if include == "" {
		return ""
	}
	for _, fill := range []string{"x", ""} {
		v := strings.NewReplacer("**", fill, "*", fill, "?", fill).Replace(filepath.ToSlash(include))
		v = strings.Trim(v, "/")
		if v != "" && environment.SecretPath(v) {
			return v
		}
	}
	return ""
}

func writeFile() *tool {
	return &tool{
		spec: provider.ToolSpec{
			Name:        "write_file",
			Description: "Create or overwrite a file with the given content. Prefer edit_file for changes to existing files.",
			InputSchema: schema([]string{"path", "content"}, map[string]any{
				"path":    str("File path, relative to the project root."),
				"content": str("The complete new content."),
			}),
		},
		kind: permission.KindEdit,
		prepare: func(s *session, input json.RawMessage) (*call, error) {
			var in struct{ Path, Content string }
			if err := decode(input, &in, "path"); err != nil {
				return nil, err
			}
			abs := s.env.Resolve(in.Path)
			return &call{title: "Write " + in.Path, paths: []string{abs}, run: func(context.Context, permission.Decision) (result, error) {
				old, err := s.env.FS().ReadFile(abs)
				if err != nil && !errors.Is(err, fs.ErrNotExist) {
					return result{}, err
				}
				if err := s.env.FS().WriteFile(abs, []byte(in.Content)); err != nil {
					return result{}, err
				}
				return result{
					text:    fmt.Sprintf("Wrote %d bytes to %s.", len(in.Content), in.Path),
					content: []any{diffContent(in.Path, string(old), in.Content)},
				}, nil
			}}, nil
		},
	}
}

func editFile() *tool {
	return &tool{
		spec: provider.ToolSpec{
			Name:        "edit_file",
			Description: "Replace an exact string in a file. old_string must match exactly once unless replace_all is true; include enough surrounding lines to make it unique.",
			InputSchema: schema([]string{"path", "old_string", "new_string"}, map[string]any{
				"path":        str("File path, relative to the project root."),
				"old_string":  str("Exact text to replace."),
				"new_string":  str("Replacement text."),
				"replace_all": flag("Replace every occurrence."),
			}),
		},
		kind: permission.KindEdit,
		prepare: func(s *session, input json.RawMessage) (*call, error) {
			var in struct {
				Path       string
				OldString  string `json:"old_string"`
				NewString  string `json:"new_string"`
				ReplaceAll bool   `json:"replace_all"`
			}
			if err := decode(input, &in, "path", "old_string"); err != nil {
				return nil, err
			}
			if in.OldString == in.NewString {
				return nil, errors.New("old_string and new_string are identical")
			}
			abs := s.env.Resolve(in.Path)
			return &call{title: "Edit " + in.Path, paths: []string{abs}, run: func(context.Context, permission.Decision) (result, error) {
				b, err := s.env.FS().ReadFile(abs)
				if err != nil {
					return result{}, err
				}
				old := string(b)
				from, to := matchLineEndings(old, in.OldString, in.NewString)
				n := strings.Count(old, from)
				switch {
				case n == 0:
					return result{}, fmt.Errorf("old_string not found in %s", in.Path)
				case n > 1 && !in.ReplaceAll:
					return result{}, fmt.Errorf("old_string occurs %d times in %s; add context to make it unique or set replace_all", n, in.Path)
				}
				updated := strings.Replace(old, from, to, map[bool]int{true: -1, false: 1}[in.ReplaceAll])
				if err := s.env.FS().WriteFile(abs, []byte(updated)); err != nil {
					return result{}, err
				}
				return result{
					text:    fmt.Sprintf("Replaced %d occurrence(s) in %s.", map[bool]int{true: n, false: 1}[in.ReplaceAll], in.Path),
					content: []any{diffContent(in.Path, old, updated)},
				}, nil
			}}, nil
		},
	}
}

// matchLineEndings fits an edit to a file's line endings (NFR-XPLAT-003). A model
// writes \n, and a file checked out on Windows often ends its lines with \r\n: in
// a file whose every line ends so, both strings get \r\n, so the edit matches and
// the lines it adds end like the others. In a file with both endings, the strings
// as written are tried first, then with \r\n. Elsewhere they are as written.
func matchLineEndings(file, from, to string) (string, string) {
	crlf := strings.Count(file, "\r\n")
	if crlf == 0 || !strings.Contains(from+to, "\n") {
		return from, to
	}
	if crlf != strings.Count(file, "\n") && strings.Contains(file, from) {
		return from, to
	}
	withCRLF := func(s string) string {
		return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
	}
	if crlf != strings.Count(file, "\n") && !strings.Contains(file, withCRLF(from)) {
		return from, to
	}
	return withCRLF(from), withCRLF(to)
}

func shellTool() *tool {
	return &tool{
		spec: provider.ToolSpec{
			Name:        "shell",
			Description: "Run a shell command in the project root (login shell). Returns the exit code and combined output, capped at 30 KB.",
			InputSchema: schema([]string{"command"}, map[string]any{
				"command":         str("The command line."),
				"timeout_seconds": num("Timeout in seconds. Default 120, maximum 600."),
				"raw":             flag("Run the command exactly as written, with its complete output. Use it when an optimizer the user chose has shortened output you need in full."),
			}),
		},
		kind: permission.KindExecute,
		prepare: func(s *session, input json.RawMessage) (*call, error) {
			var in struct {
				Command string
				Timeout int    `json:"timeout_seconds"`
				Outside bool   `json:"outside_sandbox"`
				Why     string `json:"why"`
				Raw     bool   `json:"raw"`
			}
			if err := decode(input, &in, "command"); err != nil {
				return nil, err
			}
			c := &call{title: in.Command, command: in.Command}
			spec := environment.ExecSpec{Command: in.Command, Timeout: time.Duration(in.Timeout) * time.Second}
			sandboxed := environment.IsSandboxed(s.env)
			switch {
			case sandboxed && in.Outside:
				// Once only: no rule or "always" covers leaving the sandbox.
				c.leaves, spec.OutsideSandbox = true, true
				c.options = []permission.Option{
					{ID: "allow_once", Name: "Allow once, outside the sandbox", Kind: "allow_once"},
					{ID: "reject_once", Name: "Deny", Kind: "reject_once"},
				}
				c.title = "Outside the sandbox: " + in.Command
				if in.Why != "" {
					c.title += " (" + in.Why + ")"
				}
			case sandboxed && s.Mode() == permission.ModePlan:
				c.contained, spec.ReadOnly = permission.ContainedReadOnly, true
			case sandboxed:
				c.contained = permission.ContainedProject
			}
			c.run = func(ctx context.Context, _ permission.Decision) (result, error) {
				// The broker approved in.Command; a rewrite never reaches it, and
				// a command the user let out of the sandbox runs as they read it.
				via, shown := "", ""
				if s.p.Optimizer != nil && !c.leaves {
					rw := s.p.Optimizer.Rewrite(ctx, in.Command, in.Raw)
					spec.Command, spec.Env, via, shown = rw.Command, rw.Env, rw.Via, rw.Shown
				}
				res, err := s.env.Exec(ctx, spec)
				if err != nil {
					return result{}, err
				}
				var sb strings.Builder
				fmt.Fprintf(&sb, "Exit code %d", res.ExitCode)
				if res.TimedOut {
					sb.WriteString(" (timed out; the process group was killed)")
				}
				sb.WriteString("\n")
				sb.WriteString(res.Output)
				if res.Truncated {
					sb.WriteString("\n[output truncated]")
				}
				if via != "" && res.ExitCode == 127 {
					fmt.Fprintf(&sb, "\n[This ran through %s, which may be missing. Call shell again with raw: true to run the command as written.]", via)
				}
				if c.contained != "" && res.ExitCode != 0 && sandboxRefusal(res.Output) {
					sb.WriteString(sandboxNote)
				}
				output := map[string]any{"type": "terminal_output", "exitCode": res.ExitCode, "output": res.Output}
				if via != "" {
					output["via"], output["command"] = via, shown
				}
				return result{text: sb.String(), content: []any{output}}, nil
			}
			return c, nil
		},
	}
}

// sandboxedShellSpec is the shell tool as a sandboxed session sees it.
func sandboxedShellSpec(reach string) provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "shell",
		Description: "Run a shell command in the project root (login shell), in a sandbox: it can write only in the project, read little outside it, and reach " + reach + ". Returns the exit code and combined output, capped at 30 KB.",
		InputSchema: schema([]string{"command"}, map[string]any{
			"command":         str("The command line."),
			"timeout_seconds": num("Timeout in seconds. Default 120, maximum 600."),
			"outside_sandbox": map[string]any{"type": "boolean", "description": "Ask the user to run this one command outside the sandbox, only after the sandbox has refused something it really needs, such as installing a package."},
			"why":             str("With outside_sandbox: why it needs to, for the user."),
			"raw":             flag("Run the command exactly as written, with its complete output. Use it when an optimizer the user chose has shortened output you need in full."),
		}),
	}
}

const sandboxNote = "\n[This ran in a sandbox, which may have refused it. If it really needs the internet or files outside the project, call shell again with outside_sandbox: true and why; the user decides.]"

// sandboxRefusal reports output that looks like the sandbox said no.
func sandboxRefusal(output string) bool {
	for _, s := range []string{"Operation not permitted", "Could not resolve host", "ENOTFOUND", "getaddrinfo", "nodename nor servname", "no such host", "CONNECT tunnel failed", "list of hosts"} {
		if strings.Contains(output, s) {
			return true
		}
	}
	return false
}

// loadSkill reads an enabled skill (SKL-004): the body of its SKILL.md and
// the list of its files, or one of those files. Only names and descriptions
// are in the system prompt; the rest is read on demand. Scripts are run with
// the shell tool, which asks like any command.
func loadSkill() *tool {
	return &tool{
		spec: provider.ToolSpec{
			Name:        "load_skill",
			Description: "Load a skill listed under Skills in the system prompt: returns its instructions and the files it contains. Pass file to read one of those files.",
			InputSchema: schema([]string{"name"}, map[string]any{
				"name": str("The skill's name, as listed."),
				"file": str("Optional: a file inside the skill, relative to its folder, such as references/guide.md."),
			}),
		},
		kind: permission.KindRead, readOnly: true,
		prepare: func(s *session, input json.RawMessage) (*call, error) {
			var in struct{ Name, File string }
			if err := decode(input, &in, "name"); err != nil {
				return nil, err
			}
			if s.p.Skills == nil {
				return nil, errors.New("this session has no skills")
			}
			title := "Load skill " + in.Name
			if in.File != "" {
				title += ": " + in.File
			}
			return &call{
				title: title, permTool: "skill:" + in.Name,
				run: func(context.Context, permission.Decision) (result, error) {
					c, err := s.p.Skills.Load(in.Name, in.File)
					if err != nil {
						return result{}, err
					}
					text := c.Text
					if in.File == "" {
						text = fmt.Sprintf("<skill name=%q folder=%q>\n%s\n</skill>\n", in.Name, c.Dir, strings.TrimSpace(c.Text))
						if len(c.Files) > 0 {
							text += "\nFiles in the skill (read them with load_skill and file; run scripts with the shell tool from the folder above):\n- " + strings.Join(c.Files, "\n- ") + "\n"
						}
					}
					return result{text: truncate(text, maxToolOutput), content: []any{textContent(fmt.Sprintf("Loaded %d characters", len(text)))}}, nil
				},
			}, nil
		},
	}
}

// presentPlan ends plan mode (HRN-009): the user approves the plan and picks
// the mode to continue in, or asks for more planning.
func presentPlan() *tool {
	return &tool{
		spec: provider.ToolSpec{
			Name:        "present_plan",
			Description: "Present your plan to the user for approval. Only available in plan mode. If approved, you can start making changes.",
			InputSchema: schema([]string{"plan"}, map[string]any{"plan": str("The plan, in Markdown.")}),
		},
		kind: permission.KindSwitchMode, readOnly: true,
		prepare: func(s *session, input json.RawMessage) (*call, error) {
			var in struct{ Plan string }
			if err := decode(input, &in, "plan"); err != nil {
				return nil, err
			}
			return &call{
				title: "Plan ready for review",
				options: []permission.Option{
					{ID: "approve_ask", Name: "Approve, ask before edits", Kind: "allow_once"},
					{ID: "approve_accept_edits", Name: "Approve and accept edits", Kind: "allow_once"},
					{ID: "keep_planning", Name: "Keep planning", Kind: "reject_once"},
				},
				run: func(_ context.Context, d permission.Decision) (result, error) {
					mode := permission.ModeAsk
					if d.OptionID == "approve_accept_edits" {
						mode = permission.ModeAcceptEdits
					}
					s.SetMode(mode)
					return result{text: fmt.Sprintf("The user approved the plan. The session is now in %s mode; go ahead.", mode)}, nil
				},
			}, nil
		},
	}
}

// askUser asks the user questions with options to choose from (HRN-021), shown as a
// form in the conversation; the answers come back as the tool's result.
func askUser() *tool {
	option := map[string]any{"type": "object", "required": []string{"label"}, "properties": map[string]any{
		"label":       str("What the user picks, a few words."),
		"description": str("What the option means or leads to."),
	}}
	question := map[string]any{"type": "object", "required": []string{"question", "options"}, "properties": map[string]any{
		"question":    str("The question, ending with a question mark."),
		"header":      str("A label of a word or two."),
		"options":     map[string]any{"type": "array", "items": option, "description": "2 to 8 choices; the user can always type an answer of their own."},
		"multiSelect": map[string]any{"type": "boolean", "description": "Whether the user may choose more than one."},
	}}
	return &tool{
		spec: provider.ToolSpec{
			Name:        "ask_user",
			Description: "Ask the user 1 to 4 questions with choices, when you need a decision only they can make. Do not use it to ask for approval of a plan or a change.",
			InputSchema: schema([]string{"questions"}, map[string]any{"questions": map[string]any{"type": "array", "items": question}}),
		},
		kind: permission.KindOther, readOnly: true,
		prepare: func(s *session, input json.RawMessage) (*call, error) {
			var in struct{ Questions []harness.Question }
			// CheckQuestions requires them: decode's check is for text fields.
			if err := decode(input, &in); err != nil {
				return nil, err
			}
			if err := harness.CheckQuestions(in.Questions); err != nil {
				return nil, err
			}
			message, form := harness.QuestionForm(in.Questions)
			return &call{
				title:   "Asking for your input",
				options: permission.FormOptions,
				form:    &permission.Form{Server: "The agent", Message: message, Schema: form},
				run: func(_ context.Context, d permission.Decision) (result, error) {
					return result{text: "The user answered:\n" + harness.QuestionAnswers(in.Questions, d.Answers)}, nil
				},
			}, nil
		},
	}
}

// taskTool hands a self-contained task to a subagent (HRN-014): a child
// session with the same folder, profile, model and mode, whose own tool
// calls go through the permission broker like any session's. The tool
// itself does nothing but start it, so it needs no approval; the result is
// the child's final reply, and its transcript stays open to the user.
func taskTool() *tool {
	return &tool{
		spec: provider.ToolSpec{
			Name:        "task",
			Description: "Hand a self-contained task to a subagent that works in the same project with the same tools, and get back its final answer. Use it for research or work that would fill your own context, such as surveying a large codebase. The subagent cannot see this conversation: put everything it needs in the prompt.",
			InputSchema: schema([]string{"description", "prompt"}, map[string]any{
				"description": str("A short title for the task, three to six words."),
				"prompt":      str("The full instructions for the subagent, with every detail it needs."),
			}),
		},
		kind: permission.KindThink, readOnly: true,
		prepare: func(s *session, input json.RawMessage) (*call, error) {
			var in struct{ Description, Prompt string }
			if err := decode(input, &in, "description", "prompt"); err != nil {
				return nil, err
			}
			if s.p.Subagents == nil {
				return nil, errors.New("subagents are not available in this session")
			}
			return &call{
				title: "Subagent: " + in.Description,
				run: func(ctx context.Context, _ permission.Decision) (result, error) {
					res, err := s.p.Subagents.Run(ctx, in.Description, in.Prompt)
					if err != nil {
						return result{}, err
					}
					text := strings.TrimSpace(res.Text)
					if text == "" {
						text = "The subagent finished without a reply."
					}
					return result{text: text, content: []any{
						map[string]any{"type": "content", "content": map[string]any{"type": "resource_link", "uri": "freeloop://session/" + res.SessionID, "name": "The subagent's transcript"}},
						textContent(truncate(text, 4000)),
					}}, nil
				},
			}, nil
		},
	}
}
