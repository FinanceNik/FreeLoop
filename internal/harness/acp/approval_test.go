package acp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	sdk "github.com/coder/acp-go-sdk"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/permission"
)

// Review finding: a read the policy allowed without asking left an
// approval that let any number of writes to the file through, even in
// plan mode. An approval now covers only its own kind, and a write spends
// it.
func TestApprovalCoversOnlyItsKind(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	file := filepath.Join(root, "package.json")
	os.WriteFile(file, []byte("{}"), 0o644)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root)})
	if err != nil {
		t.Fatal(err)
	}
	broker := permission.NewBroker(policy, func(string, event.Type, any) {})
	s := &session{p: harness.Params{SessionID: "x", Broker: broker, Emit: func(event.Type, any) {}}, env: env, mode: permission.ModePlan, terms: map[string]*terminal{}}
	allow := []sdk.PermissionOption{{OptionId: "ok", Kind: sdk.PermissionOptionKindAllowOnce, Name: "ok"}}
	read := sdk.ToolKindRead
	if _, err := s.RequestPermission(context.Background(), sdk.RequestPermissionRequest{
		ToolCall: sdk.ToolCallUpdate{ToolCallId: "t1", Kind: &read, Locations: []sdk.ToolCallLocation{{Path: file}}},
		Options:  allow,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteTextFile(context.Background(), sdk.WriteTextFileRequest{Path: file, Content: "changed"}); err == nil {
		t.Error("a read's approval let a write through in plan mode")
	}
	if b, _ := os.ReadFile(file); string(b) != "{}" {
		t.Errorf("file = %q", b)
	}

	// An edit approval covers one write.
	s.mode = permission.ModeAcceptEdits
	edit := sdk.ToolKindEdit
	s.RequestPermission(context.Background(), sdk.RequestPermissionRequest{
		ToolCall: sdk.ToolCallUpdate{ToolCallId: "t2", Kind: &edit, Locations: []sdk.ToolCallLocation{{Path: file}}},
		Options:  allow,
	})
	if !s.covered([]permission.Kind{permission.KindEdit}, env.Resolve(file), "", true) {
		t.Fatal("the edit's approval does not cover its write")
	}
	if s.covered([]permission.Kind{permission.KindEdit}, env.Resolve(file), "", true) {
		t.Error("the edit's approval covered a second write")
	}
}
