package permission

import (
	"regexp"
	"slices"
	"strings"
)

// safeLine is what a command that only reads may be spelled with: no expansion,
// substitution, redirection or operator but &&, and no glob.
var safeLine = regexp.MustCompile(`^[A-Za-z0-9_./:=,@%+ '"-]+( && [A-Za-z0-9_./:=,@%+ '"-]+)*$`)

// readOnlyGit are git's commands that only read, without options that write.
var readOnlyGit = []string{"status", "diff", "log", "show", "rev-parse", "ls-files", "blame"}

// neverReadOnly are options that make these programs write or run something.
var neverReadOnly = map[string][]string{
	"git":  {"--output", "--ext-diff", "--textconv", "-c", "--exec-path", "--open-files-in-pager", "-O"},
	"find": {"-delete", "-exec", "-execdir", "-ok", "-okdir", "-fprint", "-fprint0", "-fprintf", "-fls"},
	"rg":   {"--pre", "--hostname-bin"},
	"grep": {"--devices"},
	"diff": {"--output"},
	"tail": {"-f", "-F"},
	"ls":   {},
	"cat":  {},
	"head": {},
	"wc":   {},
}

// ReadOnlyCommand reports a command that certainly only reads inside the project:
// listed programs, without options that write or run, naming no path above or
// outside it. It is for a token optimizer's rewrite in an agent that asks about
// anything it does not recognize: the agent runs git status without asking, and
// would ask about the same command with a program in front of it (HRN-019). What
// it does not recognize is not read-only: the broker then asks.
func ReadOnlyCommand(line string) bool {
	if !safeLine.MatchString(line) {
		return false
	}
	commands, ok := shellWords(line)
	if !ok {
		return false
	}
	for _, fields := range commands {
		if len(fields) == 0 {
			return false
		}
		bad, known := neverReadOnly[fields[0]]
		if !known {
			return false
		}
		args := fields[1:]
		if fields[0] == "git" && (len(args) == 0 || !slices.Contains(readOnlyGit, args[0])) {
			return false
		}
		for _, word := range args {
			if leavesProject(word) || forbiddenOption(word, bad) {
				return false
			}
		}
	}
	return true
}

// shellWords reads a line safeLine let through as the shell will: quotes end up
// joined to what touches them, so `--pr"e=x"` is --pre=x, and only a && outside
// them separates commands. Judging the line as written would judge something
// else than what runs. It reports an unterminated quote.
func shellWords(line string) (commands [][]string, ok bool) {
	var words []string
	var word strings.Builder
	var quote rune
	started, quoted := false, false
	end := func() {
		switch {
		case !started:
		case word.String() == "&&" && !quoted:
			commands = append(commands, words)
			words = nil
		default:
			words = append(words, word.String())
		}
		word.Reset()
		started, quoted = false, false
	}
	for _, c := range line {
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else {
				word.WriteRune(c)
			}
		case c == '\'' || c == '"':
			quote, started, quoted = c, true, true
		case c == ' ':
			end()
		default:
			word.WriteRune(c)
			started = true
		}
	}
	if quote != 0 {
		return nil, false
	}
	end()
	return append(commands, words), true
}

// leavesProject reports a word that names, or may name, a path above or outside
// the project: alone, or as an option's value in --file=/x, -f/x or -rf/x.
func leavesProject(word string) bool {
	if strings.HasPrefix(word, "-") && !strings.HasPrefix(word, "--") {
		// Which letters of a short option take a value differs by program.
		return strings.Contains(word, "/") || strings.Contains(word, "..")
	}
	if strings.HasPrefix(word, "/") {
		return true
	}
	for _, sep := range []string{"=/", ":/", ",/", "@/", "%/", "+/"} {
		if strings.Contains(word, sep) {
			return true
		}
	}
	isSep := func(c rune) bool { return c == '/' || c == '=' || c == ':' || c == ',' }
	return slices.Contains(strings.FieldsFunc(word, isSep), "..")
}

// forbiddenOption reports one of bad, however it is spelled: with a value after =,
// a short one with its value attached, and a long one cut short, which git accepts.
func forbiddenOption(word string, bad []string) bool {
	name, _, _ := strings.Cut(word, "=")
	if name == "--" {
		return false
	}
	for _, b := range bad {
		switch {
		case name == b:
			return true
		case strings.HasPrefix(name, "--") && strings.HasPrefix(b, name):
			return true
		case len(b) == 2 && b != "--" && strings.HasPrefix(word, b):
			return true
		}
	}
	return false
}
