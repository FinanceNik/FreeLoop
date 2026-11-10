package engine

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// What can be changed in a running session (PRV-014): the same list for every
// harness, so the GUI has one set of controls (HRN-001).

const (
	optionModelID  = "model"
	optionEffortID = "effort"
	// A model's value is its provider and name; a provider ID has no such byte.
	modelSeparator = "\n"
	modelListFor   = 5 * time.Minute
	modelListWait  = 5 * time.Second
)

var efforts = []string{"low", "medium", "high", "xhigh", "max"}

// SessionOptionsReport is what a session offers. Unknown means its agent has to
// start before it can say; Note says why there is nothing to choose.
type SessionOptionsReport struct {
	Options []harness.Option `json:"options"`
	Unknown bool             `json:"unknown"`
	Note    string           `json:"note"`
}

// modelList is a provider's models as listed a moment ago.
type modelList struct {
	at     time.Time
	models []string
}

func sessionEffort(ses session.Session, prof config.Profile) string {
	return cmp.Or(ses.Effort, prof.Effort, "high")
}

// lastOptions are the choices an agent last reported, to put back when it starts.
func lastOptions(events []event.Event) []harness.OptionValue {
	options := optionsInEvents(events)
	out := make([]harness.OptionValue, 0, len(options))
	for _, o := range options {
		out = append(out, harness.OptionValue{ID: o.ID, Value: o.Current})
	}
	return out
}

func optionsInEvents(events []event.Event) []harness.Option {
	for _, ev := range slices.Backward(events) {
		if ev.Type != event.ConfigOptionUpdate {
			continue
		}
		var d struct {
			ConfigOptions json.RawMessage `json:"configOptions"`
		}
		if json.Unmarshal(ev.Data, &d) == nil {
			return harness.ParseOptions(d.ConfigOptions)
		}
	}
	return nil
}

// SessionOptions lists what can be changed in a session. It starts nothing: an
// agent that has not run yet waits for LoadSessionOptions.
func (e *Engine) SessionOptions(ctx context.Context, sid string) (SessionOptionsReport, error) {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return SessionOptionsReport{}, err
	}
	if ses.HarnessID == builtin.ID {
		return SessionOptionsReport{Options: e.builtinOptions(ctx, ses)}, nil
	}
	e.mu.Lock()
	r := e.runtimes[sid]
	e.mu.Unlock()
	if r != nil {
		return e.agentOptions(ses, r), nil
	}
	events, err := e.store.Events(ctx, sid, 0)
	if err != nil {
		return SessionOptionsReport{}, err
	}
	if options := optionsInEvents(events); len(options) > 0 {
		return SessionOptionsReport{Options: options}, nil
	}
	return SessionOptionsReport{Options: []harness.Option{}, Unknown: true}, nil
}

// LoadSessionOptions starts a session's agent, on the user's asking, so it can
// say what it offers.
func (e *Engine) LoadSessionOptions(ctx context.Context, sid string) (SessionOptionsReport, error) {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return SessionOptionsReport{}, err
	}
	if ses.HarnessID == builtin.ID {
		return e.SessionOptions(ctx, sid)
	}
	r, err := e.runtimeFor(ctx, sid)
	if err != nil {
		return SessionOptionsReport{}, err
	}
	return e.agentOptions(ses, r), nil
}

func (e *Engine) agentOptions(ses session.Session, r *runtime) SessionOptionsReport {
	report := SessionOptionsReport{Options: []harness.Option{}}
	if c, ok := r.hs.(harness.Configurable); ok {
		report.Options = c.Options()
	}
	if len(report.Options) == 0 {
		name := ses.HarnessID
		if p, err := e.preset(ses.HarnessID); err == nil {
			name = p.Name
		}
		report.Note = name + " does not offer a model or effort to choose here; it uses its own settings."
	}
	return report
}

// SetSessionOption changes one of a session's options; the next model call uses
// it. An external agent starts if it is not running.
func (e *Engine) SetSessionOption(ctx context.Context, sid, id, value string) error {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return err
	}
	if ses.HarnessID != builtin.ID {
		r, err := e.runtimeFor(ctx, sid)
		if err != nil {
			return err
		}
		c, ok := r.hs.(harness.Configurable)
		if !ok {
			return errors.New("this agent has no options to change")
		}
		return c.SetOption(ctx, id, value)
	}
	switch id {
	case optionModelID:
		providerID, model, ok := strings.Cut(value, modelSeparator)
		if !ok {
			return fmt.Errorf("not a model: %q", value)
		}
		return e.SetModel(ctx, sid, providerID, model)
	case optionEffortID:
		return e.setEffort(ctx, ses, value)
	}
	return fmt.Errorf("no option %q", id)
}

func (e *Engine) setEffort(ctx context.Context, ses session.Session, effort string) error {
	if !slices.Contains(efforts, effort) {
		return fmt.Errorf("not an effort: %q", effort)
	}
	if prof, err := e.profileFor(e.sessionProject(ctx, ses), ses.ProfileID); err == nil && sessionEffort(ses, prof) == effort {
		return nil
	}
	e.mu.Lock()
	r := e.runtimes[ses.ID]
	e.mu.Unlock()
	if r != nil {
		if s, ok := r.hs.(harness.EffortSetter); ok {
			s.SetEffort(effort)
		}
	}
	e.emit(ses.ID, event.EffortChanged, event.EffortChangedData{Effort: effort})
	return nil
}

// builtinOptions are the built-in harness's model and, where the model takes one,
// its effort. The models are those the session's provider lists, and what the
// user marked or set as the default on the others, so opening a session asks one
// provider at most.
func (e *Engine) builtinOptions(ctx context.Context, ses session.Session) []harness.Option {
	return e.modelOptions(ctx, ses, false)
}

// modelOptions are builtinOptions; with every set, the model choices hold the listed
// models of every provider that can be called unasked, not only the session's, its
// own provider's first (WFL-011). The listings are asked side by side, each cached.
func (e *Engine) modelOptions(ctx context.Context, ses session.Session, every bool) []harness.Option {
	model := harness.Option{
		ID: optionModelID, Name: "Model", Category: harness.OptionModel,
		Current: ses.ProviderID + modelSeparator + ses.Model, Custom: true,
	}
	add := func(group, providerID, name string) {
		value := providerID + modelSeparator + name
		if name != "" && !slices.ContainsFunc(model.Choices, func(c harness.OptionChoice) bool { return c.Value == value }) {
			model.Choices = append(model.Choices, harness.OptionChoice{Value: value, Name: name, Group: group})
		}
	}
	providers := slices.DeleteFunc(e.ListProviders(), func(p ProviderInfo) bool {
		return p.AddressChanged || (p.ID != ses.ProviderID && !canCallUnasked(p))
	})
	// The session's own provider first.
	slices.SortStableFunc(providers, func(a, b ProviderInfo) int {
		return cmp.Compare(boolRank(a.ID != ses.ProviderID), boolRank(b.ID != ses.ProviderID))
	})
	listed := make([][]string, len(providers))
	var wg sync.WaitGroup
	for i, p := range providers {
		if p.ID == ses.ProviderID || every {
			wg.Add(1)
			go func() {
				defer wg.Done()
				listed[i] = e.listedModels(ctx, p.ID)
			}()
		}
	}
	wg.Wait()
	for i, p := range providers {
		if p.ID == ses.ProviderID {
			add(p.Name, p.ID, ses.Model)
		}
		for _, name := range p.Favorites {
			add(p.Name, p.ID, name)
		}
		add(p.Name, p.ID, p.DefaultModel)
		for _, name := range listed[i] {
			if !slices.Contains(p.Hidden, name) {
				add(p.Name, p.ID, name)
			}
		}
	}
	// A provider that is gone or changed still shows what the session has.
	add(ses.ProviderID, ses.ProviderID, ses.Model)
	out := []harness.Option{model}
	cctx, cancel := context.WithTimeout(ctx, modelListWait)
	defer cancel()
	if caps, err := e.ModelCapabilities(cctx, ses.ProviderID, ses.Model, false); err == nil && caps.Effort {
		current := cmp.Or(ses.Effort, "high")
		if prof, err := e.profileFor(e.sessionProject(ctx, ses), ses.ProfileID); err == nil {
			current = sessionEffort(ses, prof)
		}
		effort := harness.Option{ID: optionEffortID, Name: "Effort", Category: harness.OptionEffort, Current: current}
		for _, v := range efforts {
			effort.Choices = append(effort.Choices, harness.OptionChoice{Value: v, Name: v})
		}
		out = append(out, effort)
	}
	return out
}

// canCallUnasked reports a provider a model can be chosen on: it has its key, or
// needs none.
func canCallUnasked(p ProviderInfo) bool {
	if p.HasKey || p.Builtin || p.Type == "ollama" || p.Type == string(provider.TypeScripted) {
		return true
	}
	u, err := url.Parse(p.BaseURL)
	return err == nil && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}

// boolRank orders false before true.
func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

// listedModels asks a provider for its models, at most every few minutes; one
// that does not answer leaves the choices the user already has.
func (e *Engine) listedModels(ctx context.Context, providerID string) []string {
	e.mu.Lock()
	list, ok := e.modelLists[providerID]
	e.mu.Unlock()
	if ok && time.Since(list.at) < modelListFor {
		return list.models
	}
	cctx, cancel := context.WithTimeout(ctx, modelListWait)
	defer cancel()
	list = modelList{at: time.Now()}
	if models, err := e.TestProvider(cctx, providerID); err == nil {
		for _, m := range models {
			list.models = append(list.models, m.ID)
		}
	}
	e.mu.Lock()
	e.modelLists[providerID] = list
	e.mu.Unlock()
	return list.models
}

// FileInput is a text file the user attached to a message (SES-019).
type FileInput struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// attachedFiles checks files against the limits of a message's other attachments.
func attachedFiles(in []FileInput, used int) ([]harness.Resource, error) {
	if len(in) > maxMentions {
		return nil, fmt.Errorf("a message can carry %d files", maxMentions)
	}
	total := used
	var out []harness.Resource
	for _, f := range in {
		name := strings.Map(func(r rune) rune {
			if r < ' ' || r == 0x7f {
				return -1
			}
			return r
		}, filepath.Base(f.Name))
		switch {
		case name == "" || name == "." || name == "/":
			return nil, errors.New("an attached file has no name")
		case !utf8.ValidString(f.Text) || strings.ContainsRune(f.Text, 0):
			return nil, fmt.Errorf("%s is not text", name)
		case len(f.Text) > maxMentionBytes:
			return nil, fmt.Errorf("%s: %s", name, mentionTooLarge)
		case total+len(f.Text) > maxMentionTotal:
			return nil, fmt.Errorf("%s: %s", name, mentionOverTotal)
		}
		total += len(f.Text)
		out = append(out, harness.Resource{Path: name, URI: event.AttachmentURI(url.PathEscape(name)), Text: f.Text})
	}
	return out, nil
}

// OpenProjectFolder shows the folder a session works in. The path comes from the
// session, never from the page, and what opens it never runs what is in it.
func (e *Engine) OpenProjectFolder(ctx context.Context, sid string) error {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return err
	}
	if e.folderArgv == nil {
		return errors.New("this build cannot open folders")
	}
	root := ses.WorkRoot()
	if !environment.IsDir(root) {
		return missingRoot(ses)
	}
	env, err := e.localEnv(root, nil)
	if err != nil {
		return err
	}
	res, err := env.Exec(ctx, environment.ExecSpec{Argv: e.folderArgv(root), Timeout: 30 * time.Second})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("open %s: %s", root, strings.TrimSpace(res.Output))
	}
	return nil
}

// AddSessionRule adds a rule that lasts as long as the session (SEC-002). An allow
// rule here is the user's own choice in the session, as "Allow always" is.
func (e *Engine) AddSessionRule(sid string, r permission.Rule) error {
	r.Scope = permission.ScopeSession
	if err := e.rules.AddRule(sid, r); err != nil {
		return err
	}
	e.emit(sid, event.SessionRuleChanged, sessionRuleChange{Rule: r})
	return nil
}

// DeleteSessionRule removes one of a session's rules.
func (e *Engine) DeleteSessionRule(sid string, r permission.Rule) error {
	if !e.rules.deleteSession(sid, r) {
		return errors.New("no such rule in this session")
	}
	e.emit(sid, event.SessionRuleChanged, sessionRuleChange{Rule: r, Removed: true})
	return nil
}
