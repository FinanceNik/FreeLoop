package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// barrier holds each ReadFile until two are in progress, or gives up after
// a second; so reads one after another time out, and reads at once do not.
type barrier struct {
	environment.FileSystem
	mu       sync.Mutex
	inside   int
	both     chan struct{}
	timedOut bool
}

func (b *barrier) ReadFile(path string) ([]byte, error) {
	if filepath.Ext(path) == ".txt" {
		b.mu.Lock()
		b.inside++
		if b.inside == 2 {
			close(b.both)
		}
		b.mu.Unlock()
		select {
		case <-b.both:
		case <-time.After(time.Second):
			b.mu.Lock()
			b.timedOut = true
			b.mu.Unlock()
		}
	}
	return b.FileSystem.ReadFile(path)
}

type barrierEnv struct {
	environment.Environment
	fs *barrier
}

func (e barrierEnv) FS() environment.FileSystem { return e.fs }

// HRN-014: two independent read_file calls in one model turn run at once,
// and their results come back in the order the model asked.
func TestParallelReads(t *testing.T) {
	f := setup(t, permission.ModeAsk)
	os.WriteFile(filepath.Join(f.root, "a.txt"), []byte("alpha"), 0o644)
	os.WriteFile(filepath.Join(f.root, "b.txt"), []byte("beta"), 0o644)
	read := func(path string) scripted.Call {
		return scripted.Call{Name: "read_file", Input: map[string]any{"path": path}}
	}
	prov := scripted.New("p", scripted.Step{Calls: []scripted.Call{read("a.txt"), read("b.txt")}}, scripted.Step{Text: "done"})
	b := &barrier{FileSystem: f.sess.(*session).env.FS(), both: make(chan struct{})}
	p := f.sess.(*session).p
	p.Env = barrierEnv{Environment: p.Env, fs: b}
	p.Provider = prov
	sess, err := Harness{}.NewSession(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Prompt(context.Background(), harness.Prompt{Text: "read both"}); err != nil {
		t.Fatal(err)
	}
	if b.timedOut {
		t.Error("the two reads ran one after the other")
	}
	reqs := prov.Requests()
	msgs := reqs[1].Messages
	calls, results := msgs[len(msgs)-2].Parts, msgs[len(msgs)-1].Parts
	if len(results) != 2 || results[0].ToolCallID != calls[0].ToolCallID || results[1].ToolCallID != calls[1].ToolCallID ||
		results[0].Text == results[1].Text {
		b, _ := json.Marshal(results)
		t.Errorf("results = %s", b)
	}
}
