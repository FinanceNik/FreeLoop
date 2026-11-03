package engine

import (
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/acp"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// Importing an agent's own sessions (SES-020). The agent lists its sessions,
// copies the chosen one so the original stays as it was, and replays the copy,
// which the session's transcript records; the session then continues the copy
// the way it continues any external session after a restart (SES-003). All of
// it goes through the protocol, so any agent that can list and load is covered,
// and no agent's files are read. No model is called.

// AgentSession is a session an agent keeps, as the import offers it.
type AgentSession struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Folder    string    `json:"folder"`
	UpdatedAt time.Time `json:"updatedAt"`
	// SessionID is the session here that already holds it; "" when none does.
	SessionID string `json:"sessionId"`
	// Missing says its folder is gone, so it cannot be continued.
	Missing bool `json:"missing"`
}

// ImportSessionRequest is what ImportAgentSession imports.
type ImportSessionRequest struct {
	ProfileID string `json:"profileId"`
	Folder    string `json:"folder"`
	ID        string `json:"id"`
	Title     string `json:"title"`
}

// maxImportTitle bounds a title an agent made up from a first message.
const maxImportTitle = 120

// importProfile returns a profile for an external agent and its preset.
func (e *Engine) importProfile(id string) (config.Profile, config.HarnessPreset, error) {
	if id == "" {
		return config.Profile{}, config.HarnessPreset{}, errors.New("choose an agent to import from")
	}
	prof, err := e.profileFor(session.Project{}, id)
	if err != nil {
		return prof, config.HarnessPreset{}, err
	}
	if prof.Harness == "" || prof.Harness == builtin.ID {
		return prof, config.HarnessPreset{}, fmt.Errorf("%s is not an external agent, so it has no sessions of its own to import", prof.Name)
	}
	preset, err := e.runnablePreset(prof.Harness)
	return prof, preset, err
}

// AgentSessions lists an agent's sessions, newest first: those in folder, or with
// no folder those it finds by itself and those in every project here, since an
// agent may not find a session without being told its folder.
func (e *Engine) AgentSessions(ctx context.Context, profileID, folder string) ([]AgentSession, error) {
	prof, preset, err := e.importProfile(profileID)
	if err != nil {
		return nil, err
	}
	env, err := e.localEnv(e.home, nil)
	if err != nil {
		return nil, err
	}
	b, err := acp.Harness{Preset: preset}.Browse(ctx, env)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	folders := []string{folder}
	if folder == "" {
		projects, err := e.store.ListProjects(ctx)
		if err != nil {
			return nil, err
		}
		for _, p := range projects {
			folders = append(folders, p.Root)
		}
	}
	held, err := e.heldAgentSessions(ctx, prof.Harness)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []AgentSession{}
	for i, f := range folders {
		found, err := b.List(ctx, f)
		if err != nil && i == 0 {
			return nil, err
		}
		if err != nil {
			log.Printf("engine: the sessions of %s in %s: %s", preset.Name, f, e.redactText(err.Error()))
		}
		for _, s := range found {
			if seen[s.ID] {
				continue
			}
			seen[s.ID] = true
			out = append(out, AgentSession{
				ID: s.ID, Title: importTitle(s.Title), Folder: s.Folder, UpdatedAt: s.UpdatedAt,
				SessionID: held[s.ID], Missing: !environment.IsDir(s.Folder),
			})
		}
	}
	slices.SortStableFunc(out, func(a, b AgentSession) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return out, nil
}

// heldAgentSessions maps an agent's session IDs to the sessions here that imported
// them or continue them.
func (e *Engine) heldAgentSessions(ctx context.Context, harnessID string) (map[string]string, error) {
	all, err := e.store.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	held := map[string]string{}
	for _, s := range all {
		if s.HarnessID != harnessID {
			continue
		}
		if s.ExternalID != "" {
			held[s.ExternalID] = s.ID
		}
		if source, ok := strings.CutPrefix(s.ImportedFrom, harnessID+":"); ok {
			held[source] = s.ID
		}
	}
	return held, nil
}

// importTitle makes one line of what an agent calls a session.
func importTitle(title string) string {
	title = strings.Join(strings.Fields(title), " ")
	if utf8.RuneCountInString(title) <= maxImportTitle {
		return title
	}
	return strings.TrimRight(string([]rune(title)[:maxImportTitle]), " ,.;:") + "…"
}

// ImportAgentSession imports one of an agent's sessions into a new session here,
// or returns the session that already holds it.
func (e *Engine) ImportAgentSession(ctx context.Context, req ImportSessionRequest) (session.Session, error) {
	e.importMu.Lock()
	defer e.importMu.Unlock()
	prof, preset, err := e.importProfile(req.ProfileID)
	if err != nil {
		return session.Session{}, err
	}
	if req.ID == "" {
		return session.Session{}, errors.New("choose a session to import")
	}
	if !environment.IsDir(req.Folder) {
		return session.Session{}, fmt.Errorf("its folder %s no longer exists, so the session cannot be continued", req.Folder)
	}
	held, err := e.heldAgentSessions(ctx, prof.Harness)
	if err != nil {
		return session.Session{}, err
	}
	if sid := held[req.ID]; sid != "" {
		return e.store.GetSession(ctx, sid)
	}
	project, err := e.AddProject(ctx, req.Folder)
	if err != nil {
		return session.Session{}, err
	}
	envCfg, err := e.usableEnvironment(cmp.Or(project.DefaultEnvironmentID, DefaultEnvironmentID))
	if err != nil {
		return session.Session{}, err
	}
	if envCfg.Kind == string(environment.KindDocker) {
		return session.Session{}, fmt.Errorf("the environment %s is a container, which cannot see the agent's sessions; set a local environment for this project first", envCfg.Name)
	}
	vars, err := e.resolveEnv(envCfg)
	if err != nil {
		return session.Session{}, err
	}
	env, err := e.localEnv(project.Root, vars)
	if err != nil {
		return session.Session{}, err
	}
	b, err := acp.Harness{Preset: preset}.Browse(ctx, env)
	if err != nil {
		return session.Session{}, err
	}
	defer b.Close()
	if req.Title == "" {
		// The list's name for it; failing to get one only leaves the default.
		found, _ := b.List(ctx, project.Root)
		if i := slices.IndexFunc(found, func(s harness.StoredSession) bool { return s.ID == req.ID }); i >= 0 {
			req.Title = found[i].Title
		}
	}

	ses, err := e.CreateSession(ctx, CreateSessionRequest{Title: importTitle(req.Title), ProjectRoot: project.Root, ProfileID: prof.ID, EnvironmentID: envCfg.ID})
	if err != nil {
		return session.Session{}, err
	}
	continues, copied := req.ID, ""
	if b.CanFork() {
		if continues, err = b.Fork(ctx, req.ID); err != nil {
			return e.failImport(ctx, ses.ID, err)
		}
		copied = continues
	}
	how := "continues the original, so the agent adds to it and it should not be used elsewhere at the same time"
	if copied != "" {
		how = "continues a copy, and the original is unchanged"
	}
	e.emit(ses.ID, event.Notice, event.NoticeData{Message: fmt.Sprintf("Imported from %s; this session %s. Times here are of the import, and nothing below counts toward usage. What the agent had compacted away before its last compaction is not shown: it continues from its summary of that.", preset.Name, how)})
	r := &sessionReplay{e: e, sid: ses.ID, last: map[event.Type]string{}, open: map[string]bool{}}
	if err := b.Replay(ctx, continues, r.event); err != nil {
		return e.failImport(ctx, ses.ID, err)
	}
	r.finish()
	if err := e.store.SetExternalID(ctx, ses.ID, continues); err != nil {
		return e.failImport(ctx, ses.ID, err)
	}
	e.emit(ses.ID, event.SessionImported, event.SessionImportedData{Harness: prof.Harness, Source: req.ID, Copy: copied})
	return e.store.GetSession(ctx, ses.ID)
}

// failImport removes a session whose import did not finish.
func (e *Engine) failImport(ctx context.Context, sid string, cause error) (session.Session, error) {
	if err := e.DeleteSession(ctx, sid); err != nil {
		log.Printf("engine: remove the failed import %s: %s", sid, e.redactText(err.Error()))
	}
	return session.Session{}, cause
}

// sessionReplay records what an agent replays. It keeps the conversation, and
// leaves out what describes the agent now: a later start reports that again.
type sessionReplay struct {
	e    *Engine
	sid  string
	last map[event.Type]string
	// open holds tool calls with no end, which will never get one.
	open  map[string]bool
	order []string
}

func (r *sessionReplay) event(t event.Type, m map[string]any) {
	switch t {
	case event.UserMessageChunk:
		c, _ := m["content"].(map[string]any)
		if c != nil && c["type"] == "image" {
			if !r.keepImage(m, c) {
				return
			}
		}
	case event.AgentMessageChunk, event.AgentThoughtChunk:
		c, _ := m["content"].(map[string]any)
		if c == nil || c["type"] != "text" {
			return
		}
		// One message after another with nothing between runs together in the view.
		id, _ := m["messageId"].(string)
		if prev, ok := r.last[t]; ok && prev != id {
			text, _ := c["text"].(string)
			c["text"] = "\n\n" + text
		}
		r.last[t] = id
	case event.ToolCall, event.ToolCallUpdate:
		m = dropImages(m).(map[string]any)
		r.track(m)
	case event.Plan:
	default:
		return
	}
	r.e.emit(r.sid, t, m)
}

// keepImage stores a picture of a message as one of the session's attachments, so
// the transcript names it instead of carrying it (SES-015).
func (r *sessionReplay) keepImage(m, c map[string]any) bool {
	encoded, _ := c["data"].(string)
	mime, _ := c["mimeType"].(string)
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || http.DetectContentType(data) != mime {
		return false
	}
	name, err := r.e.store.SaveAttachment(r.sid, mime, data)
	if err != nil {
		log.Printf("engine: keep an imported picture of %s: %v", r.sid, err)
		return false
	}
	m["content"] = map[string]any{"type": "image", "uri": event.AttachmentURI(name), "mimeType": mime}
	return true
}

// track notes which tool calls have ended.
func (r *sessionReplay) track(m map[string]any) {
	id, _ := m["toolCallId"].(string)
	if id == "" {
		return
	}
	if _, ok := r.open[id]; !ok {
		r.order = append(r.order, id)
	}
	status, _ := m["status"].(string)
	r.open[id] = status != "completed" && status != "failed"
}

// finish ends the tool calls the session never finished, so none looks as if it
// still runs.
func (r *sessionReplay) finish() {
	for _, id := range r.order {
		if r.open[id] {
			r.e.emit(r.sid, event.ToolCallUpdate, map[string]any{"toolCallId": id, "status": "failed"})
		}
	}
}

// dropImages takes the pictures out of a tool call's content and raw output, in
// the protocol's shape and the model API's: the view shows neither, and they would
// make up most of a transcript.
func dropImages(v any) any {
	switch x := v.(type) {
	case map[string]any:
		if x["type"] == "image" && (x["data"] != nil || x["source"] != nil) {
			return map[string]any{"type": "text", "text": "[a picture, not imported]"}
		}
		for k, item := range x {
			x[k] = dropImages(item)
		}
	case []any:
		for i, item := range x {
			x[i] = dropImages(item)
		}
	}
	return v
}
