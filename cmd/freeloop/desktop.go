//go:build !server

package main

import (
	"errors"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/dock"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/FinanceNik/Freeloop/internal/engine"
)

// appMenu is Wails' default menu without the View menu's zoom items. Those would
// claim ⌘+, ⌘- and ⌘0, which the app uses: for the Font size setting
// (NFR-A11Y-002) and the attention view. Zooming the page also scales the layout,
// which the setting does not.
func appMenu() *application.Menu {
	menu := application.NewMenu()
	menu.AddRole(application.AppMenu)
	menu.AddRole(application.FileMenu)
	menu.AddRole(application.EditMenu)
	view := menu.AddSubmenu("View")
	view.AddRole(application.Reload)
	view.AddRole(application.ForceReload)
	if os.Getenv("FREELOOP_DEV") != "" {
		view.AddRole(application.OpenDevTools)
	}
	view.AddSeparator()
	view.AddRole(application.ToggleFullscreen)
	menu.AddRole(application.WindowMenu)
	menu.AddRole(application.HelpMenu)
	return menu
}

// servedBuild is false here: the page comes from the app's own URL scheme.
const servedBuild = false

// desktop shows the engine's notices as system notifications while the
// window is not focused, opens the session a notification names when it is
// clicked, and keeps the dock badge at the number of things waiting
// (GUI-008).
type desktop struct {
	eng   *engine.Engine
	app   *application.App
	win   *application.WebviewWindow
	notes *notifications.NotificationService
	dock  *dock.DockService
	asked sync.Once
	ok    bool
}

func newDesktop(eng *engine.Engine) *desktop {
	return &desktop{eng: eng, notes: notifications.New(), dock: dock.New()}
}

func (d *desktop) services() []application.Service {
	return []application.Service{application.NewService(d.notes), application.NewService(d.dock)}
}

// start wires the window; call it once the window exists.
func (d *desktop) start(app *application.App, win *application.WebviewWindow) {
	d.app, d.win = app, win
	app.Menu.Set(appMenu())
	d.notes.OnNotificationResponse(func(r notifications.NotificationResult) {
		if r.Error != nil {
			return
		}
		sid, _ := r.Response.UserInfo["sessionId"].(string)
		// A session in a window of its own comes forward there.
		if w, ok := app.Window.GetByName(sessionWindowName(sid)); ok && sid != "" {
			w.Show()
			w.Focus()
			return
		}
		win.Show()
		win.Focus()
		if sid != "" {
			app.Event.Emit(OpenSessionEventName, sid)
		}
	})
}

func (d *desktop) notice(n engine.Notice) {
	if d.win == nil || d.focused() {
		return
	}
	// Asked on the first notice rather than at launch, so a user who never
	// leaves the window is never asked.
	d.asked.Do(func() {
		ok, err := d.notes.RequestNotificationAuthorization()
		if err != nil {
			log.Printf("notifications: %v", err)
		}
		d.ok = ok
	})
	if !d.ok {
		return
	}
	err := d.notes.SendNotification(notifications.NotificationOptions{
		ID:       n.SessionID + "-" + n.Kind,
		Title:    n.Title,
		Body:     n.Body,
		ThreadID: n.SessionID,
		Data:     map[string]any{"sessionId": n.SessionID},
	})
	if err != nil {
		log.Printf("notifications: %v", err)
	}
}

// focused reports whether any of the app's windows has the focus.
func (d *desktop) focused() bool {
	for _, w := range d.app.Window.GetAll() {
		if w.IsFocused() {
			return true
		}
	}
	return false
}

// openWindow opens session sid in a window of its own (GUI-015), or brings
// its window forward. Every window gets every engine event, so both stream.
func (d *desktop) openWindow(sid, title string) error {
	if d.app == nil {
		return errors.New("the app is not running yet")
	}
	name := sessionWindowName(sid)
	if w, ok := d.app.Window.GetByName(name); ok {
		w.Show()
		w.Focus()
		return nil
	}
	d.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      name,
		Title:     title,
		Width:     900,
		Height:    760,
		MinWidth:  520,
		MinHeight: 400,
		URL:       "/?session=" + url.QueryEscape(sid),
	}).Show()
	return nil
}

// closeGone closes the own windows of sessions that no longer exist, after
// a delete, which takes a session's subagents along (GUI-015).
func (d *desktop) closeGone(exists func(sid string) bool) {
	if d.app == nil {
		return
	}
	for _, w := range d.app.Window.GetAll() {
		if sid, ok := strings.CutPrefix(w.Name(), sessionWindowName("")); ok && !exists(sid) {
			w.Close()
		}
	}
}

func (d *desktop) badge(count int) {
	if count == 0 {
		d.dock.RemoveBadge()
		return
	}
	d.dock.SetBadge(strconv.Itoa(count))
}
