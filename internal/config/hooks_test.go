package config

import "testing"

func TestParseHooks(t *testing.T) {
	b := []byte(`version: 1
hooks:
  - event: pre_tool_use
    match: shell
    command: ./check.sh
  - event: whenever
    command: x
  - event: session_start
    match: shell
    command: x
  - event: post_tool_use
    command: ""
  - event: permission_request
    command: x
    timeout: 9999
`)
	hooks, issues := ParseHooks(b, ".freeloop/hooks.yaml")
	if len(hooks) != 1 || hooks[0].Source != ".freeloop/hooks.yaml" || len(issues) != 4 || issues[0].Field != "hooks[1]" || issues[0].Line != 6 {
		t.Fatalf("hooks = %+v, issues = %+v", hooks, issues)
	}
	if !hooks[0].Matches(HookPreToolUse, "shell") || hooks[0].Matches(HookPreToolUse, "write_file") || hooks[0].Matches(HookPostToolUse, "shell") {
		t.Error("Matches")
	}
	if !(Hook{Event: HookPostToolUse, Command: "x"}).Matches(HookPostToolUse, "anything") {
		t.Error("an empty match should match every tool")
	}
	if _, issues := ParseHooks([]byte("hooks: [\n"), "f"); len(issues) != 1 {
		t.Errorf("broken YAML = %+v", issues)
	}
	if _, issues := ParseHooks([]byte("hooks:\n  - event: pre_tool_use\n    command: x\n    when: always\n"), "f"); len(issues) != 1 {
		t.Errorf("unknown field = %+v", issues)
	}
}
