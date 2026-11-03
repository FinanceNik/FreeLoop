package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/permission"
)

// What the command line prints comes from the model.
func TestOutputCannotDisguiseItself(t *testing.T) {
	if got := plainText("safe\x1b[2K‮evil​\n"); got != "safe[2Kevil\n" {
		t.Errorf("plainText = %q", got)
	}
	if got := oneLine("Run ls\nperm_1  ses_1  Run rm -rf  [allow_once]"); got != "Run ls perm_1 ses_1 Run rm -rf [allow_once]" {
		t.Errorf("oneLine = %q", got)
	}
}

// The hook stands between the agent and every command: whatever goes wrong, the
// command runs as written, and the hook never answers for the permission.
func TestOptimizerHook(t *testing.T) {
	input := `{"tool_input":{"command":"git status","description":"see"}}`
	if out := hookOutput([]string{"/nowhere/sock", "ses"}, input); out != "" {
		t.Errorf("no app: %q", out)
	}
	if out := hookOutput([]string{"/nowhere/sock"}, input); out != "" {
		t.Errorf("bad arguments: %q", out)
	}
	dir, err := os.MkdirTemp("", "hook")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "s")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	var answer atomic.Value
	answer.Store(`{"command":"made"}`)
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			var req struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			if json.NewDecoder(c).Decode(&req) == nil && req.Method == "sessions.rewriteForAgent" {
				fmt.Fprintf(c, `{"jsonrpc":"2.0","id":%d,"result":%s}`+"\n", req.ID, answer.Load())
			}
			c.Close()
		}
	}()
	for _, in := range []string{"not json", `{"tool_input":{}}`, `{"tool_input":{"command":""}}`} {
		if out := hookOutput([]string{sock, "ses"}, in); out != "" {
			t.Errorf("%s: %q", in, out)
		}
	}
	var got struct {
		HookSpecificOutput map[string]any `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(hookOutput([]string{sock, "ses"}, input)), &got); err != nil {
		t.Fatal(err)
	}
	updated, _ := got.HookSpecificOutput["updatedInput"].(map[string]any)
	if got.HookSpecificOutput["hookEventName"] != "PreToolUse" || updated["command"] != "made" || updated["description"] != "see" {
		t.Errorf("output = %+v", got)
	}
	if _, ok := got.HookSpecificOutput["permissionDecision"]; ok {
		t.Errorf("the hook decided a permission: %+v", got)
	}
	answer.Store(`{"command":""}`)
	if out := hookOutput([]string{sock, "ses"}, input); out != "" {
		t.Errorf("no rewrite: %q", out)
	}
}

func hookOutput(args []string, in string) string {
	var out strings.Builder
	runOptimizerHook(args, strings.NewReader(in), &out)
	return out.String()
}

// An external agent names its own options, so approve finds them by kind.
func TestDefaultOption(t *testing.T) {
	pending := []permission.Request{{ID: "p", Options: []permission.Option{
		{ID: "allow-once", Kind: "allow_once"},
		{ID: "reject", Kind: "reject_once"},
	}}}
	if got := defaultOption(pending, "p", false); got != "allow-once" {
		t.Errorf("approve = %q", got)
	}
	if got := defaultOption(pending, "p", true); got != "reject" {
		t.Errorf("deny = %q", got)
	}
	if got := defaultOption(nil, "gone", false); got != "allow_once" {
		t.Errorf("no such request = %q", got)
	}
}
