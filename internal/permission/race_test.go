package permission

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// slowSink is a RuleSink whose saves can be held, and a RuleSource, safe for
// concurrent use.
type slowSink struct {
	mu    sync.Mutex
	rules []Rule
	// entered, when set, is told a save has begun, which then waits for release.
	entered chan struct{}
	release chan error
}

func (s *slowSink) Rules(string) ([]Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Rule{}, s.rules...), nil
}

func (s *slowSink) AddRule(_ string, r Rule) error {
	if s.entered != nil {
		s.entered <- struct{}{}
		if err := <-s.release; err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules = append(s.rules, r)
	return nil
}

func (s *slowSink) saved() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rules)
}

// waitPending returns the one request the broker waits on.
func waitPending(t *testing.T, b *Broker) Request {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if p := b.Pending(); len(p) == 1 {
			return p[0]
		}
		time.Sleep(100 * time.Microsecond)
	}
	t.Fatal("no request pending")
	return Request{}
}

// "Allow always" cancelled while its rule is saved: the rule is saved if and only
// if the answer is the request's decision (security review, 2026-11-04).
func TestAllowAlwaysAndACancelAgree(t *testing.T) {
	for _, c := range []struct {
		name    string
		cancel  func(b *Broker, stop context.CancelFunc)
		saveErr error
	}{
		{"turn cancelled, rule saved", func(_ *Broker, stop context.CancelFunc) { stop() }, nil},
		{"session cancelled, rule saved", func(b *Broker, _ context.CancelFunc) { b.CancelSession("s", "continued elsewhere") }, nil},
		{"turn cancelled, rule not saved", func(_ *Broker, stop context.CancelFunc) { stop() }, errors.New("disk full")},
		{"session cancelled, rule not saved", func(b *Broker, _ context.CancelFunc) { b.CancelSession("s", "continued elsewhere") }, errors.New("disk full")},
	} {
		t.Run(c.name, func(t *testing.T) {
			b, _, _, root := setup(t)
			sink := &slowSink{entered: make(chan struct{}), release: make(chan error)}
			b.SetRules(sink, sink)
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			got := make(chan Decision, 1)
			go func() {
				got <- b.Request(ctx, Context{Mode: ModeAsk, Root: root}, Request{SessionID: "s", Tool: "shell", Kind: KindExecute, Command: "make test"})
			}()
			req := waitPending(t, b)
			decided := make(chan error, 1)
			go func() { decided <- b.Decide(req.ID, "allow_always", "", "") }()
			<-sink.entered // the answer is saving its rule

			c.cancel(b, stop)
			// Give a request that ends regardless the time to do so before the save
			// finishes, as it did before the fix.
			var d Decision
			ended := false
			select {
			case d = <-got:
				ended = true
			case <-time.After(200 * time.Millisecond):
			}
			if err := b.Decide(req.ID, "reject_once", "", ""); err == nil {
				t.Error("a second answer was taken while the first saved its rule")
			}
			sink.release <- c.saveErr
			if !ended {
				d = <-got
			}
			err := <-decided

			saved := sink.saved() == 1
			if saved != (d.Outcome == Allowed && d.Persisted != nil) {
				t.Errorf("rule saved = %v, but the request ended %+v", saved, d)
			}
			if saved != (err == nil) {
				t.Errorf("rule saved = %v, Decide = %v", saved, err)
			}
			if wantSaved := c.saveErr == nil; saved != wantSaved {
				t.Errorf("rule saved = %v, want %v; decision %+v", saved, wantSaved, d)
			}
			if c.saveErr != nil && d.Outcome != Cancelled {
				t.Errorf("a failed save with a cancellation waiting ended %+v", d)
			}
			if len(b.Pending()) != 0 {
				t.Error("request still pending")
			}
		})
	}
}

// The same, raced freely: answer and cancel at once, many times over. Run with -race.
func TestAllowAlwaysRacingACancel(t *testing.T) {
	n := 2000
	if testing.Short() {
		n = 200
	}
	b, _, _, root := setup(t)
	for i := range n {
		sink := &slowSink{}
		b.SetRules(sink, sink)
		ctx, stop := context.WithCancel(context.Background())
		got := make(chan Decision, 1)
		go func() {
			got <- b.Request(ctx, Context{Mode: ModeAsk, Root: root}, Request{SessionID: "s", Tool: "shell", Kind: KindExecute, Command: "make test"})
		}()
		req := waitPending(t, b)
		start := make(chan struct{})
		var wg sync.WaitGroup
		var decideErr, secondErr error
		wg.Add(3)
		go func() { defer wg.Done(); <-start; decideErr = b.Decide(req.ID, "allow_always", "", "") }()
		go func() { defer wg.Done(); <-start; secondErr = b.Decide(req.ID, "allow_always", "", "") }()
		go func() {
			defer wg.Done()
			<-start
			if i%2 == 0 {
				stop()
			} else {
				b.CancelSession("s", "continued elsewhere")
			}
		}()
		close(start)
		d := <-got
		wg.Wait()
		stop()

		saved := sink.saved()
		answered := 0
		for _, err := range []error{decideErr, secondErr} {
			if err == nil {
				answered++
			} else if !errors.Is(err, ErrUnknownRequest) {
				t.Fatalf("iteration %d: Decide = %v", i, err)
			}
		}
		if saved > 1 {
			t.Fatalf("iteration %d: %d rules saved for one request", i, saved)
		}
		if (saved == 1) != (d.Outcome == Allowed && d.Persisted != nil) {
			t.Fatalf("iteration %d: %d rules saved, but the request ended %+v", i, saved, d)
		}
		if answered != saved {
			t.Fatalf("iteration %d: %d answers taken, %d rules saved", i, answered, saved)
		}
		if d.Outcome != Allowed && d.Outcome != Cancelled {
			t.Fatalf("iteration %d: ended %+v", i, d)
		}
	}
}
