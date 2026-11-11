package permission

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

type brokenRules struct{}

func (brokenRules) Rules(string) ([]Rule, error) { return nil, errors.New("rules.yaml has errors") }

// With rules that cannot be read, a deny rule may be among them: what is let through
// unasked is only what no deny rule could be about, a plain read inside the project, and
// a mode that refuses a thing refuses it still (SEC-003). Reproduced on 2026-11-11: an
// in-project .env read was allowed unasked, and a write in plan mode was put to the user.
func TestUnreadableRulesAskAndPlanModeStillRefuses(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	b := NewBroker(NewPolicy(home, filepath.Join(home, "data")), nil)
	b.SetRules(brokenRules{}, nil)
	root := filepath.Join(home, "repo")
	read := func(rel string) Request {
		return Request{Tool: "read_file", Kind: KindRead, Paths: []string{filepath.Join(root, rel)}}
	}
	edit := Request{Tool: "write_file", Kind: KindEdit, Paths: []string{filepath.Join(root, "a.go")}}

	if d, decided := b.evaluate(Context{Mode: ModeAsk, Root: root}, read("main.go")); !decided || d.Outcome != Allowed {
		t.Errorf("a plain read inside the project: %+v, %v", d, decided)
	}
	for _, rel := range []string{".env", "config/secrets.yaml", "deploy.pem", ".aws/credentials"} {
		for _, mode := range []Mode{ModeAsk, ModeAcceptEdits, ModePlan} {
			if d, decided := b.evaluate(Context{Mode: mode, Root: root}, read(rel)); decided {
				t.Errorf("%s in %s mode with unreadable rules was decided: %+v", rel, mode, d)
			}
		}
	}
	if d, decided := b.evaluate(Context{Mode: ModePlan, Root: root}, edit); !decided || d.Outcome != Denied || d.Actor != ActorMode {
		t.Errorf("a write in plan mode with unreadable rules: %+v, %v", d, decided)
	}
	for _, mode := range []Mode{ModeAsk, ModeAcceptEdits} {
		if d, decided := b.evaluate(Context{Mode: mode, Root: root}, edit); decided {
			t.Errorf("a write in %s mode with unreadable rules was decided: %+v", mode, d)
		}
	}
}
