// Package acptest is a reference ACP agent for tests (HRN-005): on a prompt
// it reports the MCP servers it was given, asks to edit notes.txt in its
// working folder and edits it through the client when allowed, and when the
// prompt says "terminal" runs a command through the client without asking.
package acptest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sdk "github.com/coder/acp-go-sdk"
)

// EnvVar, set to 1, makes a test binary serve the agent instead of running
// its tests; see Serve.
const EnvVar = "FREELOOP_ACP_TEST_AGENT"

// AuthVar, set to 1, makes the agent refuse every prompt as not signed in
// (the protocol's -32000), as an agent with an expired login does.
const AuthVar = "FREELOOP_ACP_TEST_AUTH"

// LoadVar, set to 1, makes the agent support session/load: it replays one
// message (which a client must not record again) and says "loaded: <id>"
// on the next prompt.
const LoadVar = "FREELOOP_ACP_TEST_LOAD"

// ImageVar, set to 1, makes the agent declare that it reads images, and
// say "images: <n>" for the image blocks of each prompt (SES-015).
const ImageVar = "FREELOOP_ACP_TEST_IMAGE"

// ImportVar, set to 1, makes the agent keep two sessions it can list, copy and
// replay (SES-020). The replay asks for permission, writes a file and asks for a
// terminal, none of which a client that only reads may grant.
const ImportVar = "FREELOOP_ACP_TEST_IMPORT"

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
	conn   *sdk.AgentSideConnection
	mcp    []string
	loaded string
	forked bool
	// plugin is the local plugin folder session/new or session/load passed
	// in _meta.claudeCode.options.plugins (SKL-008).
	plugin string
	// callTokens is set when session/new asked for the SDK's own messages.
	callTokens bool
}

// asksForCalls reports a request's _meta.claudeCode.emitRawSDKMessages.
func asksForCalls(meta map[string]any) bool {
	cc, _ := meta["claudeCode"].(map[string]any)
	return cc["emitRawSDKMessages"] != nil
}

// sendCalls sends the SDK's messages for two model calls, as the real agent shapes
// them: a call of the agent's own, whose assistant message comes before its output
// is final, and a subagent's, which is only an assistant message, twice.
func (a *agent) sendCalls(ctx context.Context, sid sdk.SessionId) {
	usage := func(in, out, read, write int) map[string]any {
		return map[string]any{"input_tokens": in, "output_tokens": out, "cache_read_input_tokens": read, "cache_creation_input_tokens": write,
			"cache_creation": map[string]any{"ephemeral_5m_input_tokens": 0, "ephemeral_1h_input_tokens": write}}
	}
	message := func(id string, u map[string]any) map[string]any {
		return map[string]any{"id": id, "model": "test-model", "usage": u}
	}
	stream := func(ev map[string]any) map[string]any {
		return map[string]any{"type": "stream_event", "parent_tool_use_id": nil, "event": ev}
	}
	sub := "toolu_1"
	for _, m := range []map[string]any{
		stream(map[string]any{"type": "message_start", "message": message("m1", usage(2, 7, 100, 50))}),
		{"type": "assistant", "parent_tool_use_id": nil, "message": message("m1", usage(2, 7, 100, 50))},
		stream(map[string]any{"type": "content_block_delta", "delta": map[string]any{"type": "text_delta", "text": "hi"}}),
		{"type": "assistant", "parent_tool_use_id": sub, "message": message("s1", usage(3, 4, 0, 20))},
		{"type": "assistant", "parent_tool_use_id": sub, "message": message("s1", usage(3, 4, 0, 20))},
		stream(map[string]any{"type": "message_delta", "usage": map[string]any{"output_tokens": 192}}),
		stream(map[string]any{"type": "message_stop"}),
		{"type": "result", "modelUsage": map[string]any{"test-model[1m]": map[string]any{"inputTokens": 1005, "outputTokens": 276, "cacheReadInputTokens": 100, "cacheCreationInputTokens": 100, "cacheCreationEphemeral1hInputTokens": 70}}},
		{"type": "assistant", "parent_tool_use_id": nil, "message": map[string]any{"id": "banner", "model": "<synthetic>", "usage": usage(0, 0, 0, 0)}},
		stream(map[string]any{"type": "message_start", "message": message("m2", usage(5, 1, 200, 0))}),
	} {
		a.conn.NotifyExtension(ctx, "_claude/sdkMessage", map[string]any{"sessionId": sid, "message": m})
	}
}

// pluginOf returns the path of the first local plugin in a request's _meta.
func pluginOf(meta map[string]any) string {
	cc, _ := meta["claudeCode"].(map[string]any)
	opts, _ := cc["options"].(map[string]any)
	plugins, _ := opts["plugins"].([]any)
	for _, p := range plugins {
		if m, _ := p.(map[string]any); m["type"] == "local" && m["skipMcpDiscovery"] == true {
			path, _ := m["path"].(string)
			return path
		}
	}
	return ""
}

func (a *agent) Initialize(context.Context, sdk.InitializeRequest) (sdk.InitializeResponse, error) {
	var sessions sdk.SessionCapabilities
	if os.Getenv(ImportVar) == "1" {
		sessions = sdk.SessionCapabilities{List: &sdk.SessionListCapabilities{}, Fork: &sdk.SessionForkCapabilities{}}
	}
	return sdk.InitializeResponse{ProtocolVersion: sdk.ProtocolVersionNumber, AuthMethods: []sdk.AuthMethod{},
		AgentCapabilities: sdk.AgentCapabilities{LoadSession: os.Getenv(LoadVar) == "1" || os.Getenv(ImportVar) == "1", SessionCapabilities: sessions, McpCapabilities: sdk.McpCapabilities{Http: true},
			PromptCapabilities: sdk.PromptCapabilities{Image: os.Getenv(ImageVar) == "1"}}}, nil
}

func (a *agent) NewSession(_ context.Context, r sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	a.plugin = pluginOf(r.Meta)
	a.callTokens = asksForCalls(r.Meta)
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
	if os.Getenv(AuthVar) == "1" {
		time.Sleep(300 * time.Millisecond) // long enough for a second prompt to queue
		return sdk.PromptResponse{}, sdk.NewAuthRequired(nil)
	}
	text := r.Prompt[0].Text.Text
	say := func(s string) {
		a.conn.SessionUpdate(ctx, sdk.SessionNotification{SessionId: r.SessionId, Update: sdk.UpdateAgentMessageText(s)})
	}
	if a.loaded != "" {
		say("loaded: " + a.loaded)
	}
	say("mcp: " + strings.Join(a.mcp, ","))
	if os.Getenv(ImportVar) == "1" {
		a.conn.SessionUpdate(ctx, sdk.SessionNotification{SessionId: r.SessionId, Update: sdk.SessionUpdate{SessionInfoUpdate: &sdk.SessionSessionInfoUpdate{Title: sdk.Ptr("The agent's name for its copy (fork)")}}})
	}
	if os.Getenv(ImageVar) == "1" {
		n := 0
		for _, b := range r.Prompt {
			if b.Image != nil && b.Image.Data != "" {
				n++
			}
		}
		say(fmt.Sprintf("images: %d", n))
	}
	if a.plugin != "" {
		say("plugin: " + a.plugin)
	}
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
	if a.callTokens {
		a.sendCalls(ctx, r.SessionId)
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
func (a *agent) ListSessions(_ context.Context, r sdk.ListSessionsRequest) (sdk.ListSessionsResponse, error) {
	cwd, _ := os.Getwd()
	if r.Cwd != nil {
		cwd = *r.Cwd
	}
	// Two pages, as an agent with many sessions sends them.
	if r.Cursor == nil {
		next := "page2"
		return sdk.ListSessionsResponse{NextCursor: &next, Sessions: []sdk.SessionInfo{
			{SessionId: "old-1", Cwd: cwd, Title: sdk.Ptr("First session"), UpdatedAt: sdk.Ptr("2026-10-01T10:00:00.000Z")},
		}}, nil
	}
	return sdk.ListSessionsResponse{Sessions: []sdk.SessionInfo{
		{SessionId: "old-2", Cwd: cwd, Title: sdk.Ptr("Second session"), UpdatedAt: sdk.Ptr("2026-10-02T10:00:00.000Z")},
		{SessionId: "gone", Cwd: "/nonexistent/folder", Title: sdk.Ptr("Folder removed")},
	}}, nil
}

// UnstableForkSession copies a session.
func (a *agent) UnstableForkSession(_ context.Context, r sdk.UnstableForkSessionRequest) (sdk.UnstableForkSessionResponse, error) {
	if r.SessionId == "broken" {
		return sdk.UnstableForkSessionResponse{}, sdk.NewInternalError(nil)
	}
	a.forked = true
	return sdk.UnstableForkSessionResponse{SessionId: "fork-of-" + r.SessionId}, nil
}
func (a *agent) ResumeSession(context.Context, sdk.ResumeSessionRequest) (sdk.ResumeSessionResponse, error) {
	return sdk.ResumeSessionResponse{}, nil
}
func (a *agent) SetSessionConfigOption(context.Context, sdk.SetSessionConfigOptionRequest) (sdk.SetSessionConfigOptionResponse, error) {
	return sdk.SetSessionConfigOptionResponse{}, nil
}

func (a *agent) LoadSession(ctx context.Context, r sdk.LoadSessionRequest) (sdk.LoadSessionResponse, error) {
	a.loaded = string(r.SessionId)
	a.plugin = pluginOf(r.Meta)
	if os.Getenv(ImportVar) == "1" {
		a.replay(ctx, r.SessionId)
	}
	a.conn.SessionUpdate(ctx, sdk.SessionNotification{SessionId: r.SessionId, Update: sdk.UpdateAgentMessageText("replayed history")})
	return sdk.LoadSessionResponse{}, nil
}

// onePixel is the smallest PNG, as an agent replays a picture from a prompt.
const onePixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

// replay sends a session's history, and tries what a replay must not be able to do.
func (a *agent) replay(ctx context.Context, id sdk.SessionId) {
	send := func(u sdk.SessionUpdate) {
		a.conn.SessionUpdate(ctx, sdk.SessionNotification{SessionId: id, Update: u})
	}
	// Only the import's own load tries it, the one in the process that made the copy:
	// a session that later continues the copy has a broker that would ask the user.
	cwd, _ := os.Getwd()
	path := filepath.Join(cwd, "replay-wrote-this.txt")
	if a.forked {
		res, err := a.conn.RequestPermission(ctx, sdk.RequestPermissionRequest{
			SessionId: id,
			ToolCall:  sdk.ToolCallUpdate{ToolCallId: "ask", Title: sdk.Ptr("Write"), Kind: sdk.Ptr(sdk.ToolKindEdit), Locations: []sdk.ToolCallLocation{{Path: path}}},
			Options:   []sdk.PermissionOption{{OptionId: "yes", Name: "Allow", Kind: sdk.PermissionOptionKindAllowOnce}},
		})
		if err == nil && res.Outcome.Selected != nil {
			send(sdk.UpdateAgentMessageText("permission was granted"))
		}
		a.conn.WriteTextFile(ctx, sdk.WriteTextFileRequest{SessionId: id, Path: path, Content: "x"})
		if _, err := a.conn.CreateTerminal(ctx, sdk.CreateTerminalRequest{SessionId: id, Command: "sh", Args: []string{"-c", "touch replay-ran-this"}}); err == nil {
			send(sdk.UpdateAgentMessageText("a terminal was granted"))
		}
	}
	send(sdk.UpdateUserMessageText("first question from " + string(id)))
	send(sdk.UpdateUserMessage(sdk.ImageBlock(onePixel, "image/png")))
	first, second := sdk.UpdateAgentMessageText("first answer"), sdk.UpdateAgentMessageText("second answer")
	first.AgentMessageChunk.MessageId, second.AgentMessageChunk.MessageId = sdk.Ptr("m1"), sdk.Ptr("m2")
	send(first)
	send(second)
	send(sdk.StartToolCall("done", "Read a.txt", sdk.WithStartKind(sdk.ToolKindExecute), sdk.WithStartRawInput(map[string]any{"command": "cat a.txt"})))
	send(sdk.UpdateToolCall("done", sdk.WithUpdateStatus(sdk.ToolCallStatusCompleted), sdk.WithUpdateContent([]sdk.ToolCallContent{
		sdk.ToolContent(sdk.TextBlock("contents")), sdk.ToolContent(sdk.ImageBlock(onePixel, "image/png")),
	})))
	send(sdk.StartToolCall("open", "Never finished", sdk.WithStartKind(sdk.ToolKindExecute)))
	send(sdk.SessionUpdate{SessionInfoUpdate: &sdk.SessionSessionInfoUpdate{Title: sdk.Ptr("A title the replay must not set")}})
}
