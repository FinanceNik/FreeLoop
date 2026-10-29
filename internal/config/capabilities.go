package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"
)

// CapabilitiesFileName caches tool-calling probe results so a model is
// probed once, not on every launch.
const CapabilitiesFileName = "capabilities.yaml"

// ProbeResult is one cached probe.
type ProbeResult struct {
	Tools    bool      `yaml:"tools"`
	ProbedAt time.Time `yaml:"probedAt"`
}

// Probes maps provider ID to model ID to result.
type Probes map[string]map[string]ProbeResult

// LoadProbes reads the cache; a missing or unreadable file is empty, since
// a lost cache only means probing again.
func LoadProbes(dir string) Probes {
	b, err := os.ReadFile(filepath.Join(dir, CapabilitiesFileName))
	if err != nil {
		return Probes{}
	}
	var f struct {
		Probes Probes `yaml:"probes"`
	}
	if yaml.Unmarshal(b, &f) != nil || f.Probes == nil {
		return Probes{}
	}
	return f.Probes
}

// SaveProbes replaces the cache.
func SaveProbes(dir string, p Probes) error {
	b, err := yaml.Marshal(struct {
		Probes Probes `yaml:"probes"`
	}{p})
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(dir, CapabilitiesFileName), b); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("config: save probe results: %w", err)
	}
	return nil
}
