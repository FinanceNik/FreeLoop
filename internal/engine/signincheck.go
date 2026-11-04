package engine

import "github.com/FinanceNik/Freeloop/internal/config"

// signInWarning says when an agent's own settings choose a sign-in its preset warns
// of (OQ-018). The agent uses what its settings say, whoever starts it. A settings file
// that cannot be read says nothing here: the agent will have its own say.
func (e *Engine) signInWarning(p config.HarnessPreset) string {
	c := p.SignInCheck
	if c == nil {
		return ""
	}
	var settings map[string]any
	if _, ok, err := e.readHomeJSON(c.File, &settings); !ok || err != nil {
		return ""
	}
	for _, keys := range c.Keys {
		var at any = settings
		for _, k := range keys {
			m, _ := at.(map[string]any)
			at = m[k]
		}
		if at == c.Value {
			return c.Warning
		}
	}
	return ""
}
