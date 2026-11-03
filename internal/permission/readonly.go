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
	for _, part := range strings.Split(line, " && ") {
		fields := strings.Fields(part)
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
		for _, a := range args {
			word := strings.Trim(a, `'"`)
			if strings.HasPrefix(word, "/") || word == ".." || strings.HasPrefix(word, "../") || strings.Contains(word, "/../") {
				return false
			}
			name, _, _ := strings.Cut(word, "=")
			if slices.Contains(bad, name) {
				return false
			}
		}
	}
	return true
}
