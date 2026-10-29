// Package acptest is a reference ACP agent for tests (HRN-005): on a prompt
// it reports the MCP servers it was given, asks to edit notes.txt in its
// working folder and edits it through the client when allowed, and when the
// prompt says "terminal" runs a command through the client without asking.
package acptest

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/coder/acp-go-sdk"
)

// EnvVar, set to 1, makes a test binary serve the agent instead of running
// its tests; see Serve.
const EnvVar = "FREELOOP_ACP_TEST_AGENT"

// Serve runs the agent on stdin and stdout until the client disconnects and
// reports whether it did, so a TestMain can return early.
func Serve() bool {
	if os.Getenv(EnvVar) != "1" {
		return false
	}
	a := &agent{}
	a.conn = sdk.NewAgentSideConnection(a, os.Stdout, os.Stdin)
	<-a.conn.Done()
	return true
}

// agent is a reference ACP agent: it edits notes.txt after asking, and
// runs a command without asking when the prompt says "terminal".
type agent struct {
	conn *sdk.AgentSideConnection
	mcp  []string
}

func (a *agent) Initialize(context.Context, sdk.InitializeRequest) (sdk.InitializeResponse, error) {
	return sdk.InitializeResponse{ProtocolVersion: sdk.ProtocolVersionNumber, AuthMethods: []sdk.AuthMethod{},
		AgentCapabilities: sdk.AgentCapabilities{McpCapabilities: sdk.McpCapabilities{Http: true}}}, nil
}

func (a *agent) NewSession(_ context.Context, r sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	for _, m := range r.McpServers {
		switch {
		case m.Stdio != nil:
			a.mcp = append(a.mcp, m.Stdio.Name+"="+m.Stdio.Command)
		case m.Http != nil:
			a.mcp = append(a.mcp, m.Http.Name+"="+m.Http.Url)
		}
	}
	return sdk.NewSessionResponse{SessionId: "s1"}, nil
}

func (a *agent) Prompt(ctx context.Context, r sdk.PromptRequest) (sdk.PromptResponse, error) {
	text := r.Prompt[0].Text.Text
	say := func(s string) {
		a.conn.SessionUpdate(ctx, sdk.SessionNotification{SessionId: r.SessionId, Update: sdk.UpdateAgentMessageText(s)})
	}
	say("mcp: " + strings.Join(a.mcp, ","))
	cwd, _ := os.Getwd()
	path := filepath.Join(cwd, "notes.txt")
	a.conn.SessionUpdate(ctx, sdk.SessionNotification{SessionId: r.SessionId, Update: sdk.StartToolCall("t1", "Edit notes.txt",
		sdk.WithStartKind(sdk.ToolKindEdit), sdk.WithStartLocations([]sdk.ToolCallLocation{{Path: path}}))})
	res, err := a.conn.RequestPermission(ctx, sdk.RequestPermissionRequest{
		SessionId: r.SessionId,
		ToolCall:  sdk.ToolCallUpdate{ToolCallId: "t1", Title: sdk.Ptr("Edit notes.txt"), Kind: sdk.Ptr(sdk.ToolKindEdit), Locations: []sdk.ToolCallLocation{{Path: path}}},
		Options: []sdk.PermissionOption{
			{OptionId: "yes", Name: "Allow", Kind: sdk.PermissionOptionKindAllowOnce},
			{OptionId: "no", Name: "Deny", Kind: sdk.PermissionOptionKindRejectOnce},
		},
	})
	status := sdk.ToolCallStatusFailed
	if err == nil && res.Outcome.Selected != nil && res.Outcome.Selected.OptionId == "yes" {
		old, err := a.conn.ReadTextFile(ctx, sdk.ReadTextFileRequest{SessionId: r.SessionId, Path: path})
		if err == nil {
			if _, err := a.conn.WriteTextFile(ctx, sdk.WriteTextFileRequest{SessionId: r.SessionId, Path: path, Content: old.Content + " edited"}); err == nil {
				status = sdk.ToolCallStatusCompleted
			}
		}
	}
	a.conn.SessionUpdate(ctx, sdk.SessionNotification{SessionId: r.SessionId, Update: sdk.SessionUpdate{ToolCallUpdate: &sdk.SessionToolCallUpdate{ToolCallId: "t1", Status: &status}}})
	if strings.Contains(text, "terminal") {
		t, err := a.conn.CreateTerminal(ctx, sdk.CreateTerminalRequest{SessionId: r.SessionId, Command: "sh", Args: []string{"-c", "echo from-terminal"}})
		if err != nil {
			say("terminal refused: " + err.Error())
		} else {
			a.conn.WaitForTerminalExit(ctx, sdk.WaitForTerminalExitRequest{SessionId: r.SessionId, TerminalId: t.TerminalId})
			out, _ := a.conn.TerminalOutput(ctx, sdk.TerminalOutputRequest{SessionId: r.SessionId, TerminalId: t.TerminalId})
			say("terminal: " + strings.TrimSpace(out.Output))
		}
	}
	in, out := 10, 5
	return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn, Usage: &sdk.Usage{InputTokens: in, OutputTokens: out, TotalTokens: in + out}}, nil
}

func (a *agent) Cancel(context.Context, sdk.CancelNotification) error { return nil }
func (a *agent) SetSessionMode(ctx context.Context, r sdk.SetSessionModeRequest) (sdk.SetSessionModeResponse, error) {
	return sdk.SetSessionModeResponse{}, nil
}
func (a *agent) Authenticate(context.Context, sdk.AuthenticateRequest) (sdk.AuthenticateResponse, error) {
	return sdk.AuthenticateResponse{}, nil
}
func (a *agent) Logout(context.Context, sdk.LogoutRequest) (sdk.LogoutResponse, error) {
	return sdk.LogoutResponse{}, nil
}
func (a *agent) CloseSession(context.Context, sdk.CloseSessionRequest) (sdk.CloseSessionResponse, error) {
	return sdk.CloseSessionResponse{}, nil
}
func (a *agent) ListSessions(context.Context, sdk.ListSessionsRequest) (sdk.ListSessionsResponse, error) {
	return sdk.ListSessionsResponse{}, nil
}
func (a *agent) ResumeSession(context.Context, sdk.ResumeSessionRequest) (sdk.ResumeSessionResponse, error) {
	return sdk.ResumeSessionResponse{}, nil
}
func (a *agent) SetSessionConfigOption(context.Context, sdk.SetSessionConfigOptionRequest) (sdk.SetSessionConfigOptionResponse, error) {
	return sdk.SetSessionConfigOptionResponse{}, nil
}
