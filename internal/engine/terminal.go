package engine

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// The integrated terminal (GUI-012): shells on pseudo-terminals in a
// session's environment, a container for a Docker session. What the user
// types there is their own action, like a terminal window of their own, so
// it does not pass through the permission broker, and nothing of it reaches
// the transcript or the model. Output streams to subscribers and the last
// scrollbackCap bytes are kept, so a view that opens later shows the screen.

// scrollbackCap bounds what a terminal keeps for a view that attaches later.
const scrollbackCap = 256 << 10

// maxTerminals bounds the terminals open at once across sessions.
const maxTerminals = 32

// TerminalInfo describes an open terminal.
type TerminalInfo struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	// Where says where the shell runs: the folder, or the container.
	Where string `json:"where"`
}

// TerminalOutput is a piece of a terminal's screen output, or its end.
type TerminalOutput struct {
	ID   string `json:"id"`
	Data []byte `json:"data,omitempty"`
	// Offset is how many bytes the terminal printed before Data, so a view
	// can join the scrollback and the chunks that follow it.
	Offset int64 `json:"offset"`
	// Exited is set once, when the shell has ended.
	Exited   bool `json:"exited,omitempty"`
	ExitCode int  `json:"exitCode,omitempty"`
}

type terminal struct {
	info TerminalInfo
	t    environment.Terminal

	mu         sync.Mutex
	scrollback []byte
	written    int64
	exited     bool
	code       int
}

type terminals struct {
	mu   sync.Mutex
	open map[string]*terminal
	subs map[int]chan TerminalOutput
	next int
	seq  int
	// pumps read the shells; apart from the engine's background work,
	// which deleting a session waits for, since a shell lives on.
	pumps sync.WaitGroup
}

// maxKept bounds the terminals kept, ended ones included; the oldest ended
// one goes first.
const maxKept = 64

// OpenTerminal starts a shell in the session's environment, starting the
// session's runtime (and its container) when needed.
func (e *Engine) OpenTerminal(ctx context.Context, sid string, cols, rows int) (TerminalInfo, error) {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return TerminalInfo{}, err
	}
	if ses.Status == session.StatusArchived {
		return TerminalInfo{}, errors.New("unarchive the session to open a terminal in it")
	}
	e.terms.mu.Lock()
	n := 0
	for _, t := range e.terms.open {
		t.mu.Lock()
		if !t.exited {
			n++
		}
		t.mu.Unlock()
	}
	e.terms.mu.Unlock()
	if n >= maxTerminals {
		return TerminalInfo{}, fmt.Errorf("%d terminals are open; close one first", n)
	}
	r, err := e.runtimeFor(ctx, sid)
	if err != nil {
		return TerminalInfo{}, err
	}
	opener, ok := r.env.(environment.Terminals)
	if !ok {
		return TerminalInfo{}, errors.New("this session's environment has no terminal")
	}
	// The terminal outlives the call that opened it; Close or the
	// session's end stops it.
	t, err := opener.OpenTerminal(context.WithoutCancel(ctx), environment.TerminalSpec{Cols: cols, Rows: rows})
	if err != nil {
		return TerminalInfo{}, err
	}
	where := r.env.Root()
	if _, docker := r.env.(*environment.Docker); docker {
		where = "container · " + where
	}
	e.terms.mu.Lock()
	e.terms.seq++
	term := &terminal{info: TerminalInfo{ID: fmt.Sprintf("term_%d", e.terms.seq), SessionID: sid, Where: where}, t: t}
	if e.terms.open == nil {
		e.terms.open = map[string]*terminal{}
	}
	e.terms.open[term.info.ID] = term
	e.forgetEnded()
	e.terms.mu.Unlock()
	e.terms.pumps.Go(func() { e.pumpTerminal(term) })
	return term.info, nil
}

// pumpTerminal reads a terminal until its shell ends and hands what it
// reads to the subscribers. Reading never stops early: on macOS a shell
// cannot finish exiting while its output is unread.
func (e *Engine) pumpTerminal(term *terminal) {
	buf := make([]byte, 32<<10)
	for {
		n, err := term.t.Read(buf)
		if n > 0 {
			data := append([]byte(nil), buf[:n]...)
			term.mu.Lock()
			offset := term.written
			term.written += int64(n)
			term.scrollback = append(term.scrollback, data...)
			if over := len(term.scrollback) - scrollbackCap; over > 0 {
				term.scrollback = append([]byte(nil), term.scrollback[over:]...)
			}
			term.mu.Unlock()
			e.publishTerminal(TerminalOutput{ID: term.info.ID, Data: data, Offset: offset})
		}
		if err != nil {
			break
		}
	}
	code, _ := term.t.Wait()
	term.t.Close()
	term.mu.Lock()
	term.exited, term.code = true, code
	end := term.written
	term.mu.Unlock()
	e.publishTerminal(TerminalOutput{ID: term.info.ID, Exited: true, ExitCode: code, Offset: end})
}

// publishTerminal hands out to every subscriber; a subscriber that falls a
// long way behind loses output rather than stalling every terminal.
func (e *Engine) publishTerminal(out TerminalOutput) {
	e.terms.mu.Lock()
	defer e.terms.mu.Unlock()
	for _, ch := range e.terms.subs {
		select {
		case ch <- out:
		default:
		}
	}
}

// SubscribeTerminals returns every terminal's output as it arrives, until
// cancel is called.
func (e *Engine) SubscribeTerminals() (<-chan TerminalOutput, func()) {
	ch := make(chan TerminalOutput, 4096)
	e.terms.mu.Lock()
	if e.terms.subs == nil {
		e.terms.subs = map[int]chan TerminalOutput{}
	}
	id := e.terms.next
	e.terms.next++
	e.terms.subs[id] = ch
	e.terms.mu.Unlock()
	return ch, func() {
		e.terms.mu.Lock()
		delete(e.terms.subs, id)
		e.terms.mu.Unlock()
	}
}

func (e *Engine) terminal(id string) (*terminal, error) {
	e.terms.mu.Lock()
	defer e.terms.mu.Unlock()
	t, ok := e.terms.open[id]
	if !ok {
		return nil, fmt.Errorf("no terminal %q", id)
	}
	return t, nil
}

// TerminalScreen returns what a terminal has printed lately, for a view
// that attaches to it, and whether its shell has ended.
func (e *Engine) TerminalScreen(id string) (TerminalOutput, error) {
	t, err := e.terminal(id)
	if err != nil {
		return TerminalOutput{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return TerminalOutput{ID: id, Data: append([]byte(nil), t.scrollback...), Offset: t.written - int64(len(t.scrollback)), Exited: t.exited, ExitCode: t.code}, nil
}

// SessionTerminals lists a session's open terminals, oldest first.
func (e *Engine) SessionTerminals(sid string) []TerminalInfo {
	e.terms.mu.Lock()
	defer e.terms.mu.Unlock()
	out := []TerminalInfo{}
	for i := 1; i <= e.terms.seq; i++ {
		if t, ok := e.terms.open[fmt.Sprintf("term_%d", i)]; ok && t.info.SessionID == sid {
			out = append(out, t.info)
		}
	}
	return out
}

// WriteTerminal sends typed input to a terminal.
func (e *Engine) WriteTerminal(id string, data []byte) error {
	t, err := e.terminal(id)
	if err != nil {
		return err
	}
	_, err = t.t.Write(data)
	return err
}

// ResizeTerminal tells a terminal its new size.
func (e *Engine) ResizeTerminal(id string, cols, rows int) error {
	t, err := e.terminal(id)
	if err != nil {
		return err
	}
	if cols < 2 || rows < 1 || cols > 1000 || rows > 500 {
		return fmt.Errorf("a terminal of %d×%d is out of range", cols, rows)
	}
	return t.t.Resize(cols, rows)
}

// CloseTerminal ends a terminal's shell and forgets it.
func (e *Engine) CloseTerminal(id string) error {
	e.terms.mu.Lock()
	t, ok := e.terms.open[id]
	delete(e.terms.open, id)
	e.terms.mu.Unlock()
	if !ok {
		return fmt.Errorf("no terminal %q", id)
	}
	return t.t.Close()
}

// closeTerminals ends a session's terminals, or every one for "".
func (e *Engine) closeTerminals(sid string) {
	e.terms.mu.Lock()
	var closing []*terminal
	for id, t := range e.terms.open {
		if sid == "" || t.info.SessionID == sid {
			closing = append(closing, t)
			delete(e.terms.open, id)
		}
	}
	e.terms.mu.Unlock()
	for _, t := range closing {
		t.t.Close()
	}
}

// forgetEnded drops the oldest ended terminals beyond maxKept; e.terms.mu
// is held.
func (e *Engine) forgetEnded() {
	for len(e.terms.open) > maxKept {
		oldest, at := "", 0
		for id, t := range e.terms.open {
			t.mu.Lock()
			exited := t.exited
			t.mu.Unlock()
			n, _ := strconv.Atoi(strings.TrimPrefix(id, "term_"))
			if exited && (oldest == "" || n < at) {
				oldest, at = id, n
			}
		}
		if oldest == "" {
			return
		}
		delete(e.terms.open, oldest)
	}
}
