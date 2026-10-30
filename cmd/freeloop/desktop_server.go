//go:build server

package main

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/FinanceNik/Freeloop/internal/engine"
)

// In server mode (a browser, no app bundle) there is no notification
// center or dock; notices and the badge are dropped.
type desktop struct{}

func newDesktop(*engine.Engine) *desktop                            { return &desktop{} }
func (*desktop) services() []application.Service                    { return nil }
func (*desktop) start(*application.App, *application.WebviewWindow) {}
func (*desktop) notice(engine.Notice)                               {}
func (*desktop) badge(int)                                          {}

// In server mode a session opens in a browser tab of its own at
// /?session=<id>; the app has no windows to open.
func (*desktop) openWindow(string, string) error {
	return errors.New("session windows open only in the desktop app; in a browser, open /?session=<id> in a new tab")
}
