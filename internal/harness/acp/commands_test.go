package acp

import (
	"context"
	"encoding/json"
	"testing"

	sdk "github.com/coder/acp-go-sdk"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
)

// GUI-049: the hint an agent gives for a command's input reaches the transcript as
// the event's own type reads it, so the message box can show where the input goes.
func TestACommandsInputHintIsKept(t *testing.T) {
	type emitted struct {
		typ  event.Type
		data any
	}
	var got []emitted
	s := &session{p: harness.Params{SessionID: "x", Emit: func(t event.Type, d any) { got = append(got, emitted{t, d}) }}}
	update := sdk.SessionUpdate{AvailableCommandsUpdate: &sdk.SessionAvailableCommandsUpdate{AvailableCommands: []sdk.AvailableCommand{
		{Name: "review", Description: "Review a pull request", Input: &sdk.AvailableCommandInput{Unstructured: &sdk.UnstructuredCommandInput{Hint: "pull request number"}}},
		{Name: "init", Description: "Write AGENTS.md"},
	}}}
	if err := s.SessionUpdate(context.Background(), sdk.SessionNotification{Update: update}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].typ != event.AvailableCommandsUpdate {
		t.Fatalf("events: %+v", got)
	}
	b, _ := json.Marshal(got[0].data)
	var data event.AvailableCommandsData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.AvailableCommands) != 2 {
		t.Fatalf("commands: %s", b)
	}
	if in := data.AvailableCommands[0].Input; in == nil || in.Hint != "pull request number" {
		t.Errorf("the hint was lost: %s", b)
	}
	if data.AvailableCommands[1].Input != nil {
		t.Errorf("a command without input got one: %s", b)
	}
}
