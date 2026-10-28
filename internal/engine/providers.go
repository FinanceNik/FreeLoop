package engine

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/provider/anthropic"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// DemoProviderID is the scripted provider offered in development builds.
const DemoProviderID = "demo"

type providerRegistry struct {
	configs []config.Provider
	built   map[string]*configuredProvider
}

type configuredProvider struct {
	cfg config.Provider
	p   provider.Provider
}

// ProviderInfo is a provider as the UI sees it; it never carries the key.
type ProviderInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	BaseURL      string `json:"baseUrl"`
	DefaultModel string `json:"defaultModel"`
	HasKey       bool   `json:"hasKey"`
	// Builtin marks providers that are not in providers.yaml.
	Builtin bool `json:"builtin"`
}

func (e *Engine) loadProviders() error {
	cfgs, issues, err := config.LoadProviders(e.dataDir)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.issues = append(e.issues, issues...)
	e.providers = &providerRegistry{configs: cfgs, built: map[string]*configuredProvider{}}
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
		out = append(out, ProviderInfo{ID: c.ID, Name: c.Name, Type: c.Type, BaseURL: c.BaseURL, DefaultModel: c.DefaultModel, HasKey: c.APIKey != ""})
	}
	if e.dev {
		d := e.demoConfig()
		out = append(out, ProviderInfo{ID: d.ID, Name: d.Name, Type: d.Type, DefaultModel: d.DefaultModel, Builtin: true})
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

	var p provider.Provider
	switch provider.Type(cfg.Type) {
	case provider.TypeAnthropic:
		key := ""
		if cfg.APIKey != "" {
			ref, err := secrets.ParseRef(cfg.APIKey)
			if err != nil {
				return nil, err
			}
			if key, err = e.secrets.Get(ref); err != nil {
				return nil, fmt.Errorf("provider %s: read API key from the Keychain: %w", cfg.Name, err)
			}
			e.remember(key)
		}
		p = anthropic.New(anthropic.Options{ID: cfg.ID, BaseURL: cfg.BaseURL, APIKey: key, Headers: cfg.Headers})
	case provider.TypeScripted:
		p = scripted.Demo(cfg.ID)
	default:
		return nil, fmt.Errorf("provider %s: type %q is not supported yet", cfg.Name, cfg.Type)
	}
	cp := &configuredProvider{cfg: cfg, p: p}
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
	if req.Type != string(provider.TypeAnthropic) {
		return ProviderInfo{}, fmt.Errorf("provider type %q is not supported yet", req.Type)
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
	cfg := config.Provider{ID: req.ID, Name: req.Name, Type: req.Type, BaseURL: strings.TrimSpace(req.BaseURL), DefaultModel: strings.TrimSpace(req.DefaultModel)}
	if i >= 0 {
		cfg.APIKey, cfg.Headers = cfgs[i].APIKey, cfgs[i].Headers
	}
	if key := strings.TrimSpace(req.APIKey); key != "" {
		ref := secrets.Ref{Service: "freeloop", Account: "providers/" + req.ID}
		if err := e.secrets.Set(ref, key); err != nil {
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
	if err := config.SaveProviders(e.dataDir, cfgs); err != nil {
		return ProviderInfo{}, err
	}
	e.mu.Lock()
	e.providers.configs = cfgs
	delete(e.providers.built, cfg.ID)
	e.mu.Unlock()
	return ProviderInfo{ID: cfg.ID, Name: cfg.Name, Type: cfg.Type, BaseURL: cfg.BaseURL, DefaultModel: cfg.DefaultModel, HasKey: cfg.APIKey != ""}, nil
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
	if ref, err := secrets.ParseRef(cfgs[i].APIKey); err == nil {
		if err := e.secrets.Delete(ref); err != nil && !errors.Is(err, secrets.ErrNotFound) {
			return err
		}
	}
	cfgs = slices.Delete(cfgs, i, i+1)
	if err := config.SaveProviders(e.dataDir, cfgs); err != nil {
		return err
	}
	e.mu.Lock()
	e.providers.configs = cfgs
	delete(e.providers.built, id)
	e.mu.Unlock()
	return nil
}

// TestProvider lists a provider's models, which proves the URL and key
// (PRV-002).
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
	return models, nil
}
