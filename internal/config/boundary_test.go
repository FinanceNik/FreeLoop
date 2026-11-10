package config

import (
	"regexp"
	"strings"
	"testing"
)

// HRN-008: an agent on a subscription signs in by itself. No built-in preset hands it
// a credential: not in its arguments, and not in its environment, where only names the
// user's own environment secrets fill and the agent's own settings may appear.
func TestPresetsCarryNoCredentials(t *testing.T) {
	secretish := regexp.MustCompile(`(?i)(token|secret|password|cookie|oauth|session|bearer|sk-|ghp_|api[_-]?key)`)
	// What a preset may set: switches, not secrets. Each is checked by name.
	allowed := map[string]bool{"NO_BROWSER": true, "DEFAULT_AUTH_REQUEST": true, "OPENCODE_PERMISSION": true, "COPILOT_ALLOW_ALL": true}
	for _, p := range BuiltinPresets() {
		for _, a := range append([]string{p.Command}, p.Args...) {
			if secretish.MatchString(a) {
				t.Errorf("%s passes %q on its command line", p.ID, a)
			}
		}
		for k, v := range p.Env {
			if !allowed[k] {
				t.Errorf("%s sets %s; a preset sets only known switches", p.ID, k)
			}
			if k == "COPILOT_ALLOW_ALL" && v != "false" {
				t.Errorf("%s lets the agent approve by itself: %s=%s", p.ID, k, v)
			}
			if k == "DEFAULT_AUTH_REQUEST" && !strings.Contains(v, `"api-key"`) {
				t.Errorf("%s asks for sign-in %s", p.ID, v)
			}
		}
	}
}
