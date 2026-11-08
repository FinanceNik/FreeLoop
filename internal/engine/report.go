package engine

import (
	"context"
	"fmt"
	"regexp"
	goruntime "runtime"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
)

// ReportLogLines is how many of the log's last lines a problem report carries.
const ReportLogLines = 200

// ProblemReport is the text "Copy a report" puts on the clipboard, for an issue: the
// app's version, this computer, the session's agent, provider and model when a session is
// given, and the end of the log. It goes nowhere by itself (NFR-PRIV-001), and
// the user reads it before pasting it. The log is redacted again here, since a
// crash trace reaches the log file without passing the redaction the app's own lines
// do, and the home folder's path, which holds the user's name, reads ~.
func (e *Engine) ProblemReport(ctx context.Context, sessionID string) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Freeloop %s (%s/%s, %s)\n", AppVersion, goruntime.GOOS, goruntime.GOARCH, goruntime.Version())
	system := "unknown"
	if e.systemInfo != nil {
		system = e.systemInfo()
	}
	fmt.Fprintf(&b, "System: %s\n", system)
	if sessionID != "" {
		if ses, err := e.store.GetSession(ctx, sessionID); err == nil {
			agent := ses.HarnessID
			if agent == builtin.ID {
				agent = "Built-in"
			} else if p, err := e.preset(ses.HarnessID); err == nil && p.Name != "" {
				agent = p.Name
			}
			fmt.Fprintf(&b, "Session: %s", agent)
			if ses.ProviderID != "" {
				fmt.Fprintf(&b, " on %s", e.providerName(ses.ProviderID))
			}
			if ses.Model != "" {
				fmt.Fprintf(&b, ", model %s", ses.Model)
			}
			fmt.Fprintf(&b, ", %s mode, %s\n", ses.Mode, ses.Status)
		}
	}
	lines, err := config.LogTail(e.dataDir, ReportLogLines)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "\nThe log's last %d lines:\n", len(lines))
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return e.anonymize(e.redactText(b.String())), nil
}

// providerName is a provider's name as the user gave it, else its ID.
func (e *Engine) providerName(id string) string {
	for _, p := range e.ListProviders() {
		if p.ID == id && p.Name != "" {
			return p.Name
		}
	}
	return id
}

// anonymize writes the home folder as ~, so the user's account name stays out.
func (e *Engine) anonymize(s string) string {
	return anonymizeHome(goruntime.GOOS, e.home, s)
}

// anonymizeHome writes home in s as ~. On Windows the folder appears in several
// spellings, C:\Users\a, C:/Users/a from git, /c/Users/a from Git Bash and
// C:\\Users\\a in JSON, in any case, since the file system ignores it
// (NFR-XPLAT-003).
func anonymizeHome(goos, home, s string) string {
	if goos != "windows" {
		if home == "" || home == "/" {
			return s
		}
		return strings.ReplaceAll(s, strings.TrimRight(home, "/"), "~")
	}
	home = strings.TrimRight(home, `\/`)
	if len(home) < 3 {
		return s
	}
	slashed := strings.ReplaceAll(home, `\`, "/")
	spellings := []string{strings.ReplaceAll(home, `\`, `\\`), home, slashed, bashPath("windows", home)}
	var alts []string
	for _, sp := range spellings {
		alts = append(alts, regexp.QuoteMeta(sp))
	}
	return regexp.MustCompile("(?i)"+strings.Join(alts, "|")).ReplaceAllString(s, "~")
}
