package main

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/FinanceNik/Freeloop/internal/engine"
)

// quitGuard asks before quitting would interrupt work (GUI-027). macOS wants the
// answer to "may I quit" at once and the dialog cannot block the main thread, so
// the first answer is no; choosing Quit in the dialog quits again.
type quitGuard struct {
	app       *application.App
	eng       *engine.Engine
	confirmed atomic.Bool
	asking    atomic.Bool
}

func (q *quitGuard) shouldQuit() bool {
	if q.confirmed.Load() || q.app == nil {
		return true
	}
	c := q.eng.CheckQuit(context.Background())
	if c.Empty() {
		return true
	}
	if q.asking.CompareAndSwap(false, true) {
		go q.ask(c)
	}
	return false
}

func (q *quitGuard) ask(c engine.QuitCheck) {
	dialog := q.app.Dialog.Question().SetTitle("Quit Freeloop?").SetMessage(quitMessage(c))
	quit := dialog.AddButton("Quit")
	quit.OnClick(func() {
		q.confirmed.Store(true)
		q.app.Quit()
	})
	keep := dialog.AddButton("Keep running")
	keep.OnClick(func() { q.asking.Store(false) })
	dialog.SetDefaultButton(keep).SetCancelButton(keep).Show()
}

// watch holds the main window open while the question stands: closing the last
// window quits the app, and answering "keep running" with no window left would
// leave sessions running with nothing to show them.
func (q *quitGuard) watch(win *application.WebviewWindow) {
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if len(q.app.Window.GetAll()) == 1 && !q.shouldQuit() {
			e.Cancel()
		}
	})
}

// quitMessage says what would be interrupted.
func quitMessage(c engine.QuitCheck) string {
	count := func(n int, one string) string {
		if n == 1 {
			return "1 " + one
		}
		return fmt.Sprintf("%d %ss", n, one)
	}
	var parts []string
	if c.Working > 0 {
		parts = append(parts, count(c.Working, "session")+" still working")
	}
	if c.Waiting > 0 {
		parts = append(parts, count(c.Waiting, "approval")+" unanswered")
	}
	return strings.Join(parts, " and ") + ". Quitting stops them; you can continue each session when you open the app again."
}
