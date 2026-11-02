package engine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/provider/anthropic"
	"github.com/FinanceNik/Freeloop/internal/provider/gemini"
	"github.com/FinanceNik/Freeloop/internal/provider/ollama"
	"github.com/FinanceNik/Freeloop/internal/provider/openaicompat"
	"github.com/FinanceNik/Freeloop/internal/provider/openairesponses"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// DemoProviderID is the scripted provider offered in development builds.
const DemoProviderID = "demo"

type providerRegistry struct {
	configs []config.Provider
	built   map[string]*configuredProvider
	probes  config.Probes
}

type configuredProvider struct {
	cfg config.Provider
	p   provider.Provider
	// slots holds one token per model call allowed at once; nil means no
	// cap (OQ-013).
	slots chan struct{}
}

// DefaultMaxConcurrent is a provider's cap on simultaneous model calls.
const DefaultMaxConcurrent = 4

// ProviderInfo is a provider as the UI sees it; it never carries the key.
type ProviderInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Preset       string `json:"preset"`
	BaseURL      string `json:"baseUrl"`
	DefaultModel string `json:"defaultModel"`
	HasKey       bool   `json:"hasKey"`
	// Builtin marks providers that are not in providers.yaml.
	Builtin bool `json:"builtin"`
	// AddressChanged says the address is not the one the key is for: it changed
	// outside the app, and the key is not sent until the user confirms it.
	AddressChanged bool `json:"addressChanged"`
	// Favorites and Hidden are the user's model lists (PRV-003).
	Favorites []string `json:"favorites"`
	Hidden    []string `json:"hidden"`
}

func info(c config.Provider) ProviderInfo {
	pi := ProviderInfo{ID: c.ID, Name: c.Name, Type: c.Type, Preset: c.Preset, BaseURL: c.BaseURL, DefaultModel: c.DefaultModel, HasKey: c.APIKey != "", Favorites: []string{}, Hidden: []string{}}
	for _, m := range slices.Sorted(maps.Keys(c.Models)) {
		if c.Models[m].Favorite {
			pi.Favorites = append(pi.Favorites, m)
		}
		if c.Models[m].Hidden {
			pi.Hidden = append(pi.Hidden, m)
		}
	}
	return pi
}

// PresetInfo prefills the provider form (PRV-001).
type PresetInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	BaseURL      string `json:"baseUrl"`
	NeedsKey     bool   `json:"needsKey"`
	Local        bool   `json:"local"`
	DefaultModel string `json:"defaultModel"`
	Note         string `json:"note"`
}

// ProviderPresets lists the endpoints the provider form offers.
func (e *Engine) ProviderPresets() []PresetInfo {
	out := []PresetInfo{
		{ID: "anthropic", Name: "Anthropic", Type: string(provider.TypeAnthropic), BaseURL: anthropic.DefaultBaseURL, NeedsKey: true, DefaultModel: anthropic.DefaultModel},
		{ID: string(openairesponses.Type), Name: "OpenAI (Responses API)", Type: string(openairesponses.Type), BaseURL: openairesponses.DefaultBaseURL, NeedsKey: true, DefaultModel: openairesponses.DefaultModel,
			Note: "OpenAI's native API: reasoning summaries and reasoning kept across tool calls. The OpenAI preset below uses Chat Completions."},
		{ID: string(gemini.Type), Name: "Google Gemini", Type: string(gemini.Type), BaseURL: gemini.DefaultBaseURL, NeedsKey: true, DefaultModel: gemini.DefaultModel,
			Note: "An API key from Google AI Studio."},
		{ID: "ollama", Name: "Ollama", Type: string(ollama.Type), BaseURL: ollama.DefaultBaseURL, Local: true,
			Note: "Pull a model with tool support first, for example `ollama pull qwen3`."},
	}
	for _, p := range openaicompat.Presets {
		out = append(out, PresetInfo{ID: p.ID, Name: p.Name, Type: string(openaicompat.Type), BaseURL: p.BaseURL, NeedsKey: p.NeedsKey, Local: p.Local, Note: p.Note})
	}
	return out
}

func (e *Engine) loadProviders() error {
	cfgs, issues, err := config.LoadProviders(e.dataDir)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.issues[issueProviders] = issues
	e.providers = &providerRegistry{configs: cfgs, built: map[string]*configuredProvider{}, probes: config.LoadProbes(e.dataDir)}
	e.mu.Unlock()
	return nil
}

func (e *Engine) demoConfig() config.Provider {
	return config.Provider{ID: DemoProviderID, Name: "Scripted demo", Type: string(provider.TypeScripted), DefaultModel: "scripted"}
}

// ListProviders returns the configured providers.
func (e *Engine) ListProviders() []ProviderInfo {
	e.mu.Lock()
	cfgs := slices.Clone(e.providers.configs)
	e.mu.Unlock()
	out := []ProviderInfo{}
	for _, c := range cfgs {
		pi := info(c)
		pi.AddressChanged = e.addressChanged(c)
		out = append(out, pi)
	}
	if e.dev {
		pi := info(e.demoConfig())
		pi.Builtin = true
		out = append(out, pi)
	}
	return out
}

// provider returns a ready provider, building it on first use.
func (e *Engine) provider(id string) (*configuredProvider, error) {
	e.mu.Lock()
	if p, ok := e.providers.built[id]; ok {
		e.mu.Unlock()
		return p, nil
	}
	i := slices.IndexFunc(e.providers.configs, func(c config.Provider) bool { return c.ID == id })
	var cfg config.Provider
	switch {
	case i >= 0:
		cfg = e.providers.configs[i]
	case e.dev && id == DemoProviderID:
		cfg = e.demoConfig()
	default:
		e.mu.Unlock()
		return nil, fmt.Errorf("no provider %q; add one under Providers", id)
	}
	e.mu.Unlock()

	key := ""
	if cfg.APIKey != "" {
		var err error
		if key, err = e.providerKey(cfg); err != nil {
			return nil, err
		}
	}
	var p provider.Provider
	switch provider.Type(cfg.Type) {
	case provider.TypeAnthropic:
		p = anthropic.New(anthropic.Options{ID: cfg.ID, BaseURL: cfg.BaseURL, APIKey: key, Headers: cfg.Headers})
	case openairesponses.Type:
		p = openairesponses.New(openairesponses.Options{ID: cfg.ID, BaseURL: cfg.BaseURL, APIKey: key, Headers: cfg.Headers})
	case gemini.Type:
		p = gemini.New(gemini.Options{ID: cfg.ID, BaseURL: cfg.BaseURL, APIKey: key, Headers: cfg.Headers})
	case openaicompat.Type:
		p = openaicompat.New(openaicompat.Options{ID: cfg.ID, Preset: cfg.Preset, BaseURL: cfg.BaseURL, APIKey: key, Headers: cfg.Headers})
	case ollama.Type:
		p = ollama.New(ollama.Options{ID: cfg.ID, BaseURL: cfg.BaseURL})
	case provider.TypeScripted:
		p = scripted.Demo(cfg.ID)
	default:
		return nil, fmt.Errorf("provider %s: type %q is not supported", cfg.Name, cfg.Type)
	}
	n := cfg.MaxConcurrent
	if n <= 0 {
		n = DefaultMaxConcurrent
	}
	cp := &configuredProvider{cfg: cfg, p: p, slots: make(chan struct{}, n)}
	e.mu.Lock()
	e.providers.built[id] = cp
	e.mu.Unlock()
	return cp, nil
}

// SaveProviderRequest adds or updates a provider. APIKey is written to the
// Keychain and never stored in a file; empty keeps the current key.
type SaveProviderRequest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Preset       string `json:"preset"`
	BaseURL      string `json:"baseUrl"`
	APIKey       string `json:"apiKey"`
	DefaultModel string `json:"defaultModel"`
}

var slugChars = regexp.MustCompile(`[^a-z0-9]+`)

// SaveProvider adds a provider (empty ID) or updates one.
func (e *Engine) SaveProvider(req SaveProviderRequest) (ProviderInfo, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return ProviderInfo{}, errors.New("a provider needs a name")
	}
	switch provider.Type(req.Type) {
	case provider.TypeAnthropic, ollama.Type, openairesponses.Type, gemini.Type:
		req.Preset = ""
	case openaicompat.Type:
		if req.Preset == "" {
			req.Preset = "custom"
		}
	default:
		return ProviderInfo{}, fmt.Errorf("provider type %q is not supported", req.Type)
	}
	if req.Type == string(openaicompat.Type) && req.Preset == "custom" && strings.TrimSpace(req.BaseURL) == "" {
		return ProviderInfo{}, errors.New("a custom endpoint needs a base URL")
	}
	if err := checkAddress("the base URL", req.BaseURL); err != nil {
		return ProviderInfo{}, err
	}
	e.mu.Lock()
	cfgs := slices.Clone(e.providers.configs)
	e.mu.Unlock()

	i := -1
	if req.ID != "" {
		i = slices.IndexFunc(cfgs, func(c config.Provider) bool { return c.ID == req.ID })
		if i < 0 {
			return ProviderInfo{}, fmt.Errorf("no provider %q", req.ID)
		}
	} else {
		base := strings.Trim(slugChars.ReplaceAllString(strings.ToLower(req.Name), "-"), "-")
		if base == "" || base == DemoProviderID {
			base = "provider"
		}
		req.ID = base
		for n := 2; slices.ContainsFunc(cfgs, func(c config.Provider) bool { return c.ID == req.ID }); n++ {
			req.ID = fmt.Sprintf("%s-%d", base, n)
		}
	}
	cfg := config.Provider{ID: req.ID, Name: req.Name, Type: req.Type, Preset: req.Preset,
		BaseURL: strings.TrimSpace(req.BaseURL), DefaultModel: strings.TrimSpace(req.DefaultModel)}
	if i >= 0 {
		cfg.APIKey, cfg.Headers, cfg.Models, cfg.MaxConcurrent = cfgs[i].APIKey, cfgs[i].Headers, cfgs[i].Models, cfgs[i].MaxConcurrent
	}
	// Saving in the app is the user saying where the key is for. A key kept for
	// a changed address moves with it; a file changing the address does not.
	key := strings.TrimSpace(req.APIKey)
	if key == "" && cfg.APIKey != "" {
		stored, err := e.secrets.Get(providerRef(req.ID))
		if err != nil {
			return ProviderInfo{}, fmt.Errorf("read the API key from the Keychain: %w", err)
		}
		_, key, _ = unbindSecret(stored)
	}
	if key != "" {
		ref := providerRef(req.ID)
		if err := e.secrets.Set(ref, bindSecret(keyAddress(cfg), key)); err != nil {
			return ProviderInfo{}, fmt.Errorf("store the API key in the Keychain: %w", err)
		}
		e.remember(key)
		cfg.APIKey = ref.String()
	}
	if i >= 0 {
		cfgs[i] = cfg
	} else {
		cfgs = append(cfgs, cfg)
	}
	if err := e.saveConfigs(cfgs); err != nil {
		return ProviderInfo{}, err
	}
	if cfg.APIKey != "" {
		e.noteKeyAddress(cfg.ID, keyAddress(cfg))
	}
	return info(cfg), nil
}

// saveConfigs writes providers.yaml and drops built providers so the next
// use picks up the change.
func (e *Engine) saveConfigs(cfgs []config.Provider) error {
	if err := config.SaveProviders(e.dataDir, cfgs); err != nil {
		return err
	}
	e.mu.Lock()
	e.providers.configs = cfgs
	e.providers.built = map[string]*configuredProvider{}
	e.mu.Unlock()
	return nil
}

// DeleteProvider removes a provider and its Keychain entry.
func (e *Engine) DeleteProvider(id string) error {
	e.mu.Lock()
	cfgs := slices.Clone(e.providers.configs)
	e.mu.Unlock()
	i := slices.IndexFunc(cfgs, func(c config.Provider) bool { return c.ID == id })
	if i < 0 {
		return fmt.Errorf("no provider %q", id)
	}
	// Only its own item: what the file names might be another's.
	if cfgs[i].APIKey != "" {
		if err := e.secrets.Delete(providerRef(id)); err != nil && !errors.Is(err, secrets.ErrNotFound) {
			return err
		}
	}
	e.mu.Lock()
	delete(e.keyAddresses, id)
	e.mu.Unlock()
	if err := e.saveConfigs(slices.Delete(cfgs, i, i+1)); err != nil {
		return err
	}
	e.checkKeyAddresses()
	return nil
}

// TestProvider lists a provider's models, which proves the URL and key
// (PRV-002, PRV-003).
func (e *Engine) TestProvider(ctx context.Context, id string) ([]provider.ModelInfo, error) {
	p, err := e.provider(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	models, err := p.p.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.cfg.Name, err)
	}
	if models == nil {
		models = []provider.ModelInfo{}
	}
	return models, nil
}

// ModelCapabilities resolves what a model can do (PRV-005): the user's
// override wins, then a cached probe, then what the adapter or endpoint
// reports. With probe set, a model still unknown is probed and the result
// cached.
func (e *Engine) ModelCapabilities(ctx context.Context, providerID, model string, probe bool) (provider.ModelCaps, error) {
	p, err := e.provider(providerID)
	if err != nil {
		return provider.ModelCaps{}, err
	}
	caps := p.p.Capabilities(model)
	if caps.Source == provider.CapsUnknown {
		if d, ok := p.p.(provider.Describer); ok {
			dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			described, err := d.Describe(dctx, model)
			cancel()
			if err != nil {
				return provider.ModelCaps{}, fmt.Errorf("%s: %w", p.cfg.Name, err)
			}
			caps = described
		}
	}
	if caps.Source == provider.CapsUnknown {
		e.mu.Lock()
		cached, ok := e.providers.probes[providerID][model]
		e.mu.Unlock()
		switch {
		case ok:
			caps.Source, caps.Tools = provider.CapsProbe, cached.Tools
		case probe:
			ok, err := provider.ProbeTools(ctx, p.p, model)
			if err != nil {
				return provider.ModelCaps{}, fmt.Errorf("tool-calling probe of %s on %s: %w", model, p.cfg.Name, err)
			}
			caps.Source, caps.Tools = provider.CapsProbe, ok
			e.saveProbe(providerID, model, ok)
		}
	}
	if s, ok := p.cfg.Models[model]; ok && s.Tools != nil {
		caps.Source, caps.Tools = provider.CapsOverride, *s.Tools
	}
	return caps, nil
}

func (e *Engine) saveProbe(providerID, model string, tools bool) {
	e.mu.Lock()
	if e.providers.probes[providerID] == nil {
		e.providers.probes[providerID] = map[string]config.ProbeResult{}
	}
	e.providers.probes[providerID][model] = config.ProbeResult{Tools: tools, ProbedAt: time.Now().UTC()}
	snapshot := e.providers.probes
	e.mu.Unlock()
	if err := config.SaveProbes(e.dataDir, snapshot); err != nil {
		e.setIssues(issueCapabilities, []config.Issue{{File: config.CapabilitiesFileName, Message: err.Error()}})
	} else {
		e.setIssues(issueCapabilities, nil)
	}
}

// SetToolsOverride records the user's choice for a model's tool support;
// nil removes the override (PRV-005).
func (e *Engine) SetToolsOverride(providerID, model string, tools *bool) error {
	return e.updateModel(providerID, model, func(s *config.ModelSettings) { s.Tools = tools })
}

// SetModelFlags marks a model as a favorite or hidden; a model is not both
// (PRV-003).
func (e *Engine) SetModelFlags(providerID, model string, favorite, hidden bool) error {
	if model == "" {
		return errors.New("no model")
	}
	if favorite && hidden {
		return errors.New("a model cannot be both a favorite and hidden")
	}
	return e.updateModel(providerID, model, func(s *config.ModelSettings) { s.Favorite, s.Hidden = favorite, hidden })
}

// updateModel changes one model's settings in providers.yaml.
func (e *Engine) updateModel(providerID, model string, change func(*config.ModelSettings)) error {
	e.mu.Lock()
	cfgs := slices.Clone(e.providers.configs)
	e.mu.Unlock()
	i := slices.IndexFunc(cfgs, func(c config.Provider) bool { return c.ID == providerID })
	if i < 0 {
		return fmt.Errorf("no provider %q", providerID)
	}
	models := map[string]config.ModelSettings{}
	for k, v := range cfgs[i].Models {
		models[k] = v
	}
	s := models[model]
	change(&s)
	if s.IsZero() {
		delete(models, model)
	} else {
		models[model] = s
	}
	if len(models) == 0 {
		models = nil
	}
	cfgs[i].Models = models
	return e.saveConfigs(cfgs)
}

// requireTools fails when the built-in harness cannot use model.
func (e *Engine) requireTools(ctx context.Context, providerID, model string) error {
	caps, err := e.ModelCapabilities(ctx, providerID, model, true)
	if err != nil {
		return err
	}
	if !caps.Tools {
		how := map[provider.CapsSource]string{
			provider.CapsReported: "the endpoint reports no tool support for it",
			provider.CapsProbe:    "it did not return a tool call when tested",
			provider.CapsOverride: "tool use is turned off for it",
		}[caps.Source]
		return fmt.Errorf("model %s cannot be used by the built-in harness because %s; turn on \"Use anyway\" to override (PRV-005)", model, how)
	}
	return nil
}
