package engine

import (
	"cmp"
	"crypto/hmac"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/netguard"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/provider/anthropic"
	"github.com/FinanceNik/Freeloop/internal/provider/gemini"
	"github.com/FinanceNik/Freeloop/internal/provider/ollama"
	"github.com/FinanceNik/Freeloop/internal/provider/openaicompat"
	"github.com/FinanceNik/Freeloop/internal/provider/openairesponses"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// Commands in the app's config files (SET-005, EXT-005, HRN-005): hooks in
// config.yaml and profiles, and agent presets. A file arrives without asking,
// from a restored backup or anything else that can write to the data folder, and
// its command would run with every session, outside the broker and any sandbox.
// So one runs only with a mark from approvalMark, which needs a key the file
// cannot read. The app adds the mark when the user writes the command in the app
// or approves it in Settings; a command edited in a file waits for approval. A
// project's own hooks get their mark when the project is trusted, as one set,
// and a set that changed since waits here too (projecthooks.go, SEC-006).
//
// The same holds for what changes how much is asked or confined without running
// anything itself (SEC-002, ENV-009, SEC-001): an allow rule, an environment's
// settings, permissions.allowBypass, a profile starting in accept_edits, and a
// file choosing a token optimizer. A rule's or setting's mark says what is
// approved and where. Until then an allow rule does not apply, a session cannot
// use the environment, bypass cannot be chosen, and a session starts asking with
// no optimizer. What only tightens needs no mark, deny rules among them.
//
// And for where things are sent (SET-005, SEC-009): a provider's address, the
// search and MCP registry addresses, a profile's fallback and the update check.
// Nothing goes to one until approved. The app's own list of vendor addresses and
// this computer need no mark, as long as no key or headers of the file's go with them.

// Kinds of PendingCommand.
const (
	pendingHook        = "hook"
	pendingAgent       = "agent"
	pendingRule        = "rule"
	pendingEnvironment = "environment"
	pendingSetting     = "setting"
	pendingAddress     = "address"
	// Kinds only a restore's preview lists; each has its own approval elsewhere.
	pendingOptimizer = "optimizer"
	pendingMCP       = "mcp"
)

// PendingCommand is a command or setting from a file, as Settings shows it for
// approval.
type PendingCommand struct {
	Kind string `json:"kind"`
	// Source says where it is, Event is a hook's event, and Runs what would run or
	// change.
	Source      string `json:"source"`
	Event       string `json:"event"`
	Runs        string `json:"runs"`
	Fingerprint string `json:"fingerprint"`
}

// fileCommands lists everything the app's own files define that needs a mark.
func (e *Engine) fileCommands() []PendingCommand {
	return fileItems(e.filesNow())
}

// filesNow is what the config files say, as the app holds them.
func (e *Engine) filesNow() config.Snapshot {
	e.mu.Lock()
	s := config.Snapshot{Config: e.cfg, Environments: slices.Clone(e.envs), Profiles: slices.Clone(e.profiles), Presets: slices.Clone(e.presets), Providers: slices.Clone(e.providers.configs)}
	e.mu.Unlock()
	s.Rules = e.rules.fileRules()
	return s
}

func fileItems(s config.Snapshot) []PendingCommand {
	out := settingsItems(s.Config)
	out = append(out, ruleItems(config.RulesFileName, s.Rules)...)
	for _, c := range s.Providers {
		if item, ok := providerItem(c); ok {
			out = append(out, item)
		}
	}
	for _, c := range s.Environments {
		if !builtinEnvironment(c) {
			out = append(out, PendingCommand{Kind: pendingEnvironment, Source: environmentFile(c.ID), Runs: describeEnvironment(c), Fingerprint: c.Fingerprint()})
		}
		out = append(out, ruleItems(environmentFile(c.ID), c.Rules)...)
	}
	for _, p := range s.Profiles {
		out = append(out, profileItems(p)...)
	}
	for _, p := range s.Presets {
		if !p.Builtin {
			out = append(out, PendingCommand{Kind: pendingAgent, Source: presetFile(p), Runs: describePreset(p), Fingerprint: p.Fingerprint()})
		}
	}
	return out
}

func hookItems(source string, hooks []config.Hook) []PendingCommand {
	var out []PendingCommand
	for _, h := range hooks {
		out = append(out, PendingCommand{Kind: pendingHook, Source: source, Event: h.Event, Runs: h.Command, Fingerprint: h.Fingerprint()})
	}
	return out
}

// ruleItems lists the allow rules a file holds.
func ruleItems(where string, rules []permission.Rule) []PendingCommand {
	var out []PendingCommand
	for _, r := range rules {
		if r.Effect == permission.Allow {
			runs := r.String()
			if r.Command == "" && r.Path == "" {
				runs += ", every call of it"
			}
			out = append(out, PendingCommand{Kind: pendingRule, Source: where, Runs: runs, Fingerprint: config.RuleFingerprint(where, r)})
		}
	}
	return out
}

// settingsItems lists what config.yaml holds that needs a mark.
func settingsItems(c config.Config) []PendingCommand {
	out := hookItems(config.FileName, c.Hooks)
	if c.Permissions.AllowBypass {
		out = append(out, PendingCommand{Kind: pendingSetting, Source: config.FileName, Runs: "permissions.allowBypass: true, which lets a session run every tool call unasked", Fingerprint: config.BypassFingerprint()})
	}
	if id := c.Defaults.Optimizer; id != "" && id != config.OptimizerOff {
		out = append(out, PendingCommand{Kind: pendingSetting, Source: config.FileName, Runs: fmt.Sprintf("optimizer: %s, which runs that program on new sessions' commands", id), Fingerprint: config.DefaultOptimizerFingerprint(id)})
	}
	if u := c.Web.SearchURL; u != "" {
		out = append(out, PendingCommand{Kind: pendingAddress, Source: config.FileName, Runs: "web.searchURL: the model's searches go to " + u, Fingerprint: config.AddressFingerprint("web.searchURL", u)})
	}
	if u := c.MCP.RegistryURL; u != "" {
		out = append(out, PendingCommand{Kind: pendingAddress, Source: config.FileName, Runs: "mcp.registryURL: browsing MCP servers asks " + u, Fingerprint: config.AddressFingerprint("mcp.registryURL", u)})
	}
	if c.Updates.Check {
		out = append(out, PendingCommand{Kind: pendingAddress, Source: config.FileName, Runs: "updates.check: true, which asks GitHub for a newer release once a day", Fingerprint: config.UpdatesFingerprint()})
	}
	return out
}

// providerItem says what a provider sends where, if that needs a mark. A vendor's
// address the app ships and this computer are no surprise, unless something of the
// file's goes along: headers can carry someone else's key, so that the vendor
// keeps the prompts in their account, and a key is for an address the user chose.
func providerItem(c config.Provider) (PendingCommand, bool) {
	address := providerAddress(c)
	shipped := shippedAddress(c)
	switch {
	case c.Type == string(provider.TypeScripted):
		return PendingCommand{}, false
	case len(c.Headers) == 0 && address != "" && address == shipped:
		return PendingCommand{}, false
	case len(c.Headers) == 0 && c.APIKey == "" && localAddress(address):
		return PendingCommand{}, false
	}
	shown := cmp.Or(c.BaseURL, shipped)
	runs := fmt.Sprintf("%s sends prompts and the files they carry to %s", c.Name, shown)
	if len(c.Headers) > 0 {
		runs += ", with the headers " + strings.Join(slices.Sorted(maps.Keys(c.Headers)), ", ")
	}
	return PendingCommand{Kind: pendingAddress, Source: config.ProvidersFileName, Runs: runs, Fingerprint: c.AddressFingerprint(address)}, true
}

// providerAddress is the origin a provider sends to.
func providerAddress(c config.Provider) string {
	if strings.TrimSpace(c.BaseURL) != "" {
		return originOf(c.BaseURL)
	}
	return shippedAddress(c)
}

// shippedAddress is the origin the app ships for a provider's type and preset; ""
// for one it has none for, as a custom endpoint.
func shippedAddress(c config.Provider) string {
	base := ""
	switch provider.Type(c.Type) {
	case provider.TypeAnthropic:
		base = anthropic.DefaultBaseURL
	case openairesponses.Type:
		base = openairesponses.DefaultBaseURL
	case gemini.Type:
		base = gemini.DefaultBaseURL
	case ollama.Type:
		base = ollama.DefaultBaseURL
	case openaicompat.Type:
		if i := slices.IndexFunc(openaicompat.Presets, func(p openaicompat.Preset) bool { return p.ID == c.Preset }); i >= 0 {
			base = openaicompat.Presets[i].BaseURL
		}
	}
	if base == "" {
		return ""
	}
	return originOf(base)
}

// localAddress reports an origin on this computer, by the check local-only mode uses.
func localAddress(origin string) bool {
	u, err := url.Parse(origin)
	return err == nil && u.Host != "" && netguard.Local(u.Hostname())
}

// providerBlocked says why a provider may not send anything; "" when it may.
func (e *Engine) providerBlocked(c config.Provider) string {
	item, ok := providerItem(c)
	if !ok || e.commandApproved(item.Fingerprint) {
		return ""
	}
	return fmt.Sprintf("provider %s: nothing is sent to %s until you approve it in Settings", c.Name, cmp.Or(c.BaseURL, "its address"))
}

// markProvider records that the user chose where a provider sends.
func (e *Engine) markProvider(c config.Provider) error {
	item, ok := providerItem(c)
	if !ok {
		return nil
	}
	mark, err := e.approvalMark(item.Fingerprint)
	if err != nil {
		return err
	}
	return e.addApprovals([]string{mark})
}

// profileItems lists what a profile holds that needs a mark.
func profileItems(p config.Profile) []PendingCommand {
	out := hookItems("profile "+p.Name, p.Hooks)
	out = append(out, ruleItems(profileFile(p.ID), p.Rules)...)
	if p.Mode == string(permission.ModeAcceptEdits) {
		out = append(out, PendingCommand{Kind: pendingSetting, Source: profileFile(p.ID), Runs: "mode: accept_edits, which starts sessions writing files in the project unasked", Fingerprint: p.ModeFingerprint()})
	}
	if p.Optimizer != "" && p.Optimizer != config.OptimizerOff {
		out = append(out, PendingCommand{Kind: pendingSetting, Source: profileFile(p.ID), Runs: fmt.Sprintf("optimizer: %s, which runs that program on its sessions' commands", p.Optimizer), Fingerprint: p.OptimizerFingerprint()})
	}
	if f := p.Fallback; f != nil {
		out = append(out, PendingCommand{Kind: pendingAddress, Source: profileFile(p.ID), Runs: fmt.Sprintf("fallback: calls that keep failing go to %s, model %s", f.Provider, f.Model), Fingerprint: p.FallbackFingerprint()})
	}
	return out
}

// fallbackOf is a profile's fallback, once approved.
func (e *Engine) fallbackOf(p config.Profile) *config.ModelRef {
	if p.Fallback == nil || !e.commandApproved(p.FallbackFingerprint()) {
		return nil
	}
	return p.Fallback
}

// startOptimizer is the optimizer a session starts with when none was asked for:
// the profile's, then the default. A file's choice waits for approval, and the
// session starts with none until then, as it does with off.
func (e *Engine) startOptimizer(p config.Profile) string {
	if p.Optimizer != "" {
		if p.Optimizer == config.OptimizerOff || !e.commandApproved(p.OptimizerFingerprint()) {
			return config.OptimizerOff
		}
		return p.Optimizer
	}
	id := e.Config().Defaults.Optimizer
	if id == "" || !e.commandApproved(config.DefaultOptimizerFingerprint(id)) {
		return config.OptimizerOff
	}
	return id
}

// StartWaits says what a new session of a profile will not start with yet, because
// a file chose it and the user has not approved it, for the new-session form.
type StartWaits struct {
	Mode      bool `json:"mode"`
	Optimizer bool `json:"optimizer"`
}

// StartWaiting tells the new-session form what a profile's sessions start without.
func (e *Engine) StartWaiting(profileID string) StartWaits {
	p, err := e.profile(profileID)
	if err != nil {
		return StartWaits{}
	}
	waits := StartWaits{Mode: e.startMode(p) != p.Mode}
	switch {
	case p.Optimizer == config.OptimizerOff:
	case p.Optimizer != "":
		waits.Optimizer = e.startOptimizer(p) == config.OptimizerOff
	default:
		waits.Optimizer = e.Config().Defaults.Optimizer != "" && e.startOptimizer(p) == config.OptimizerOff
	}
	return waits
}

// saveChoosing saves settings in which the user just chose what fingerprint
// names, which counts even if a file already said the same.
func (e *Engine) saveChoosing(c config.Config, fingerprint string) (config.Config, error) {
	marks, err := e.writtenMarks(settingsItems(c))
	if err != nil {
		return config.Config{}, err
	}
	mark, err := e.approvalMark(fingerprint)
	if err != nil {
		return config.Config{}, err
	}
	return e.saveSettings(c, append(marks, mark))
}

func environmentFile(id string) string { return filepath.Join(config.EnvironmentsDir, id+".yaml") }

func profileFile(id string) string { return filepath.Join(config.ProfilesDir, id+".yaml") }

// builtinEnvironment reports a file that says only what a built-in environment
// says anyway, as saving one's name writes.
func builtinEnvironment(c config.Environment) bool {
	switch c.ID {
	case DefaultEnvironmentID:
		return c.Fingerprint() == defaultEnvironment().Fingerprint()
	case SandboxedEnvironmentID:
		return c.Fingerprint() == sandboxedEnvironment().Fingerprint()
	}
	return false
}

// environmentApproved reports whether an environment's settings may apply.
func (e *Engine) environmentApproved(c config.Environment) bool {
	return builtinEnvironment(c) || e.commandApproved(c.Fingerprint())
}

// describeEnvironment is what approving an environment allows, all of it: the
// settings its fingerprint covers.
func describeEnvironment(c config.Environment) string {
	parts := []string{c.Kind}
	add := func(label, value string) {
		if value != "" {
			parts = append(parts, label+" "+value)
		}
	}
	add("image", c.Image)
	if c.Kind == string(environment.KindSSH) {
		parts = append(parts, "copies the project to "+c.Host+" and runs commands there")
		add("under", c.RemoteDir)
	}
	if c.Devcontainer {
		parts = append(parts, "the project's devcontainer.json")
	}
	add("setup", c.Setup)
	add("health check", c.Health)
	add("starting in", c.DefaultMode)
	if c.Kind == "local" {
		if c.SandboxOn() {
			parts = append(parts, "sandboxed")
			add("network", c.Sandbox.Network)
			add("hosts", strings.Join(c.Sandbox.Hosts, ", "))
			add("also reads", strings.Join(c.Sandbox.Read, ", "))
			add("also writes", strings.Join(c.Sandbox.Write, ", "))
		} else {
			parts = append(parts, "not sandboxed")
		}
	}
	var vars []string
	for _, k := range slices.Sorted(maps.Keys(c.Env)) {
		vars = append(vars, k+"="+c.Env[k])
	}
	add("sets", strings.Join(vars, ", "))
	add("gets secrets", strings.Join(slices.Sorted(maps.Keys(c.Secrets)), ", "))
	return strings.Join(parts, "; ")
}

// ruleApproved reports whether an allow rule from a file may apply.
func (e *Engine) ruleApproved(r permission.Rule) bool {
	return e.commandApproved(config.RuleFingerprint(ruleFile(r), r))
}

// ruleFile says which file a loaded rule is from, wherever the data folder is.
func ruleFile(r permission.Rule) string {
	if r.Scope == permission.ScopeGlobal {
		return config.RulesFileName
	}
	return r.Source
}

// approveRule records that the user added a global allow rule in the app.
func (e *Engine) approveRule(r permission.Rule) error {
	mark, err := e.approvalMark(config.RuleFingerprint(config.RulesFileName, r))
	if err != nil {
		return err
	}
	return e.addApprovals([]string{mark})
}

// revokeRule ends the approval of a global allow rule.
func (e *Engine) revokeRule(r permission.Rule) error {
	return e.revoke(config.RuleFingerprint(config.RulesFileName, r))
}

// revoke ends an approval.
func (e *Engine) revoke(fingerprint string) error {
	mark, err := e.approvalMark(fingerprint)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.cfg.CommandApprovals = slices.DeleteFunc(slices.Clone(e.cfg.CommandApprovals), func(m string) bool { return m == mark })
	e.mu.Unlock()
	_, err = e.saveSettings(e.Config(), nil)
	return err
}

// startMode is the mode a profile starts sessions in. A file's accept_edits
// waits for approval, and sessions start asking until then.
func (e *Engine) startMode(p config.Profile) string {
	if p.Mode == string(permission.ModeAcceptEdits) && !e.commandApproved(p.ModeFingerprint()) {
		return ""
	}
	return p.Mode
}

// guards lists the deny rules and blocking hooks in config files, which a restored
// file can leave out without anything waiting for approval.
func guards(s config.Snapshot) []string {
	var out []string
	rules := func(where string, list []permission.Rule) {
		for _, r := range list {
			if r.Effect == permission.Deny {
				out = append(out, r.String()+", in "+where)
			}
		}
	}
	hooks := func(where string, list []config.Hook) {
		for _, h := range list {
			if h.Event == config.HookPreToolUse || h.Event == config.HookPermissionRequest {
				out = append(out, "the "+h.Event+" hook "+h.Command+", in "+where)
			}
		}
	}
	rules(config.RulesFileName, s.Rules)
	hooks(config.FileName, s.Config.Hooks)
	for _, c := range s.Environments {
		rules(environmentFile(c.ID), c.Rules)
	}
	for _, p := range s.Profiles {
		rules(profileFile(p.ID), p.Rules)
		hooks(profileFile(p.ID), p.Hooks)
	}
	return out
}

func presetFile(p config.HarnessPreset) string {
	return filepath.Join(config.PresetsDir, p.ID+".yaml")
}

// describePreset is what approving a preset allows to run.
func describePreset(p config.HarnessPreset) string {
	quoted := func(s string) string {
		if strings.ContainsAny(s, " \t\n'\"") {
			return shellQuote(s)
		}
		return s
	}
	parts := []string{quoted(p.Command)}
	for _, a := range p.Args {
		parts = append(parts, quoted(a))
	}
	out := strings.Join(parts, " ")
	var vars []string
	for _, k := range slices.Sorted(maps.Keys(p.Env)) {
		vars = append(vars, k+"="+p.Env[k])
	}
	if len(vars) > 0 {
		out += ", with " + strings.Join(vars, ", ") + " set"
	}
	return out
}

// PendingCommands lists the commands from files waiting for the user's approval,
// trusted projects' changed hooks among them.
func (e *Engine) PendingCommands() []PendingCommand {
	out := []PendingCommand{}
	for _, c := range append(e.fileCommands(), e.projectHookItems()...) {
		if !e.commandApproved(c.Fingerprint) {
			out = append(out, c)
		}
	}
	return out
}

// commandApproved reports whether the user approved exactly this command.
func (e *Engine) commandApproved(fingerprint string) bool {
	return e.markIn(e.Config().CommandApprovals, fingerprint)
}

// markIn reports whether approvals hold this computer's mark for fingerprint.
func (e *Engine) markIn(approvals []string, fingerprint string) bool {
	mark, err := e.approvalMark(fingerprint)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(approvals, func(m string) bool {
		return hmac.Equal([]byte(mark), []byte(m))
	})
}

// ApproveCommand approves a command as the user saw it, which must still be what
// a file says.
func (e *Engine) ApproveCommand(fingerprint string) (config.Config, error) {
	return e.ApproveCommands([]string{fingerprint})
}

// ApproveCommands approves the list as the user saw it, or nothing if any of it is
// no longer what a file says. It names what to approve, so what arrived since the
// user looked keeps waiting.
func (e *Engine) ApproveCommands(fingerprints []string) (config.Config, error) {
	pending := e.PendingCommands()
	marks := make([]string, 0, len(fingerprints))
	for _, fingerprint := range fingerprints {
		if !slices.ContainsFunc(pending, func(c PendingCommand) bool { return c.Fingerprint == fingerprint }) {
			return config.Config{}, errors.New("that command changed or is gone; look at it again")
		}
		mark, err := e.approvalMark(fingerprint)
		if err != nil {
			return config.Config{}, err
		}
		marks = append(marks, mark)
	}
	return e.saveSettings(e.Config(), marks)
}

// writtenMarks are the marks for what the user is saving in the app. A form sends
// back all it holds, so what still waits for approval, here or in another list,
// stays waiting: changing a setting or copying a profile does not approve what a
// backup brought.
func (e *Engine) writtenMarks(items []PendingCommand) ([]string, error) {
	waiting := map[[2]string]bool{}
	for _, c := range e.PendingCommands() {
		waiting[[2]string{c.Kind, c.Runs}] = true
	}
	var marks []string
	for _, c := range items {
		if waiting[[2]string{c.Kind, c.Runs}] || e.commandApproved(c.Fingerprint) {
			continue
		}
		mark, err := e.approvalMark(c.Fingerprint)
		if err != nil {
			return nil, err
		}
		marks = append(marks, mark)
	}
	return marks, nil
}

// addApprovals records marks in config.yaml.
func (e *Engine) addApprovals(marks []string) error {
	if len(marks) == 0 {
		return nil
	}
	_, err := e.saveSettings(e.Config(), marks)
	return err
}

// approvedHooks keeps the hooks the user approved and says once per session
// which did not run: one may be a guard the user counts on.
func (e *Engine) approvedHooks(r *runtime, hooks []config.Hook) []config.Hook {
	var out []config.Hook
	for _, h := range hooks {
		fp := h.Fingerprint()
		if e.commandApproved(fp) {
			out = append(out, h)
			continue
		}
		r.callsMu.Lock()
		seen := r.unapproved[fp]
		if r.unapproved == nil {
			r.unapproved = map[string]bool{}
		}
		r.unapproved[fp] = true
		r.callsMu.Unlock()
		if !seen {
			e.emit(r.sid, event.Notice, event.NoticeData{Message: fmt.Sprintf("A hook from %s does not run, because it has not been approved as it is now: %s. Approve it in Settings.", h.Source, capText(h.Command))})
		}
	}
	return out
}

// runnablePreset returns a preset whose command may run.
func (e *Engine) runnablePreset(id string) (config.HarnessPreset, error) {
	p, err := e.preset(id)
	if err != nil {
		return p, err
	}
	if why := e.presetBlocked(p); why != "" {
		return p, fmt.Errorf("%s cannot start: %s", p.Name, why)
	}
	return p, nil
}

// presetBlocked says why a preset's command may not run; "" when it may.
func (e *Engine) presetBlocked(p config.HarnessPreset) string {
	if p.Builtin || e.commandApproved(p.Fingerprint()) {
		return ""
	}
	return fmt.Sprintf("%s has not been approved as it is now; approve it in Settings", presetFile(p))
}

// pendingIssues lists what waits for approval among the problems the app shows.
func (e *Engine) pendingIssues() []config.Issue {
	var out []config.Issue
	for _, c := range e.PendingCommands() {
		msg := "%s does not run until you approve it in Settings"
		if c.Kind != pendingHook && c.Kind != pendingAgent && c.Kind != pendingProjectHooks {
			msg = "%s does not apply until you approve it in Settings"
		}
		out = append(out, config.Issue{File: c.Source, Message: fmt.Sprintf(msg, c.Runs), Approval: true})
	}
	return out
}

// commandsAdoptedRef marks that a data folder's existing commands were adopted.
// It is in the Keychain because a backup can replace any config file, and per
// folder because a scratch one (FREELOOP_HOME) says nothing about the real one.
func (e *Engine) commandsAdoptedRef() secrets.Ref {
	return secrets.Ref{Service: "freeloop", Account: "app/commands-adopted/" + e.dataDir}
}

// settingsAdoptedRef is the same for rules, environments and settings, which came
// to need marks later: a Mac that adopted its commands has not adopted these, and
// a command waiting there must not be approved by these being adopted.
func (e *Engine) settingsAdoptedRef() secrets.Ref {
	return secrets.Ref{Service: "freeloop", Account: "app/settings-adopted/" + e.dataDir}
}

// addressesAdoptedRef is the same for addresses and the update check, for the same
// reason.
func (e *Engine) addressesAdoptedRef() secrets.Ref {
	return secrets.Ref{Service: "freeloop", Account: "app/addresses-adopted/" + e.dataDir}
}

// adoptExistingCommands approves, once for each kind, what the files hold when
// the first version with its approvals starts, so what a user already has keeps
// working. It applied before that too, so this trusts nothing new; a backup
// restored from then on finds the item there.
func (e *Engine) adoptExistingCommands() {
	e.adopt(e.commandsAdoptedRef(), pendingHook, pendingAgent)
	e.adopt(e.settingsAdoptedRef(), pendingRule, pendingEnvironment, pendingSetting)
	e.adopt(e.addressesAdoptedRef(), pendingAddress)
	e.adoptProjectHooks()
}

// adopt writes the item first: if the rest fails the user is asked, which is the
// safe way to be wrong.
func (e *Engine) adopt(ref secrets.Ref, kinds ...string) {
	if _, err := e.secrets.Get(ref); !errors.Is(err, secrets.ErrNotFound) {
		return
	}
	if err := e.secrets.Set(ref, "1"); err != nil {
		log.Printf("engine: adopt what the files hold: %v", err)
		return
	}
	var marks []string
	for _, c := range e.fileCommands() {
		if !slices.Contains(kinds, c.Kind) {
			continue
		}
		mark, err := e.approvalMark(c.Fingerprint)
		if err != nil {
			log.Printf("engine: adopt what the files hold: %v", err)
			return
		}
		marks = append(marks, mark)
	}
	if err := e.addApprovals(marks); err != nil {
		log.Printf("engine: adopt what the files hold: %v", err)
	}
}
