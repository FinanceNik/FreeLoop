package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// Tailscale help for sessions from the phone (GUI-052): the app reads, never
// changes, the user's Tailscale setup, and shows the command that makes the phone
// server reachable on their tailnet.

// tailscalePaths are where Tailscale's command is when a desktop app's PATH does not
// have it: the Mac app's own, Windows', and the usual Linux ones.
var tailscalePaths = []string{
	"/Applications/Tailscale.app/Contents/MacOS/Tailscale",
	"/usr/local/bin/tailscale",
	"/opt/homebrew/bin/tailscale",
	"/usr/bin/tailscale",
	`C:\Program Files\Tailscale\tailscale.exe`,
}

// TailscaleInfo is what the phone settings show about Tailscale.
type TailscaleInfo struct {
	Found bool   `json:"found"`
	Path  string `json:"path"`
	// Running says the computer is connected to its tailnet.
	Running bool `json:"running"`
	// Name is this computer's name on the tailnet, as the phone reaches it.
	Name string `json:"name"`
	// HTTPS says the tailnet has HTTPS certificates turned on.
	HTTPS bool `json:"https"`
	// Serving says something is already served on the port the command would use.
	Serving bool   `json:"serving"`
	Command string `json:"command"`
	Undo    string `json:"undo"`
	Error   string `json:"error,omitempty"`
}

// PhoneHTTPSPort is the tailnet port the command serves the phone server on: not
// 443, which the user may already serve something on.
const PhoneHTTPSPort = 8443

// Tailscale looks at this computer's Tailscale, reading only.
func (e *Engine) Tailscale(ctx context.Context) TailscaleInfo {
	port := e.Config().Remote.RemotePort()
	info := TailscaleInfo{}
	env, err := e.localEnv(e.home, nil)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	program := locateProgram(ctx, env, "tailscale")
	for _, p := range tailscalePaths {
		if program != "" {
			break
		}
		if environment.IsFile(p) {
			program = p
		}
	}
	if program == "" {
		return info
	}
	info.Found, info.Path = true, program
	quoted := program
	if strings.ContainsAny(quoted, " \\") {
		quoted = `"` + quoted + `"`
	}
	info.Command = fmt.Sprintf("%s serve --bg --https=%d http://127.0.0.1:%d", quoted, PhoneHTTPSPort, port)
	info.Undo = fmt.Sprintf("%s serve --https=%d off", quoted, PhoneHTTPSPort)
	run := func(args ...string) (string, bool) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		res, err := env.Exec(ctx, environment.ExecSpec{Argv: append([]string{program}, args...), MaxOutput: 1 << 20})
		if err != nil || res.ExitCode != 0 {
			return "", false
		}
		return res.Output, true
	}
	out, ok := run("status", "--json")
	if !ok {
		info.Error = "tailscale status did not answer; is Tailscale running?"
		return info
	}
	var st struct {
		BackendState string
		CertDomains  []string
		Self         struct {
			DNSName string
		}
	}
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		info.Error = err.Error()
		return info
	}
	info.Running = st.BackendState == "Running"
	info.Name = strings.TrimSuffix(st.Self.DNSName, ".")
	info.HTTPS = len(st.CertDomains) > 0
	if out, ok := run("serve", "status", "--json"); ok {
		info.Serving = strings.Contains(out, ":"+strconv.Itoa(PhoneHTTPSPort)+`"`)
	}
	return info
}
