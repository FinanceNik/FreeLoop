//go:build !server

package main

import (
	"cmp"
	"errors"
	"log"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/dock"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/FinanceNik/Freeloop/internal/engine"
)

// appMenu is Wails' default menu without the View menu's zoom items, in lang
// (GUI-048). Those would claim ⌘+, ⌘- and ⌘0, which the app uses: for the Font size
// setting (NFR-A11Y-002) and the attention view. Zooming the page also scales the
// layout, which the setting does not. Its Help menu opens the app's own help, where
// Wails' would load its website into the window.
func appMenu(lang string, help func()) *application.Menu {
	menu := application.NewMenu()
	menu.AddRole(application.AppMenu)
	menu.AddRole(application.FileMenu)
	menu.AddRole(application.EditMenu)
	view := menu.AddSubmenu("View")
	view.AddRole(application.Reload)
	view.AddRole(application.ForceReload)
	if devEnv("FREELOOP_DEV") != "" {
		view.AddRole(application.OpenDevTools)
	}
	view.AddSeparator()
	view.AddRole(application.ToggleFullscreen)
	menu.AddRole(application.WindowMenu)
	menu.AddSubmenu("Help").Add("Freeloop Help").OnClick(func(*application.Context) { help() })
	localizeMenu(menu, lang, thisPlatform.MenuWords)
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
	// lang is the interface's language, as the window resolved it (GUI-048).
	mu   sync.Mutex
	lang string
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
	app.Menu.Set(appMenu(d.language(), d.openHelp))
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

// language is the interface's language, English until the window says.
func (d *desktop) language() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return cmp.Or(d.lang, "en")
}

// setLanguage puts the menu and the notifications in lang, the interface's language
// as the window resolved it; the menu is built again when it changes.
func (d *desktop) setLanguage(lang string) {
	if !slices.Contains(interfaceLanguages, lang) {
		lang = "en"
	}
	d.mu.Lock()
	changed := d.lang != lang
	d.lang = lang
	d.mu.Unlock()
	if changed && d.app != nil {
		d.app.Menu.Set(appMenu(lang, d.openHelp))
	}
}

// openHelp brings the main window forward on the help.
func (d *desktop) openHelp() {
	if d.win == nil {
		return
	}
	d.win.Show()
	d.win.Focus()
	d.app.Event.Emit(OpenHelpEventName, "")
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
	title, body := noticeText(n, d.language())
	err := d.notes.SendNotification(notifications.NotificationOptions{
		ID:       n.SessionID + "-" + n.Kind,
		Title:    title,
		Body:     body,
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
		Name:        name,
		Title:       title,
		Width:       900,
		Height:      760,
		MinWidth:    520,
		MinHeight:   400,
		URL:         "/?session=" + url.QueryEscape(sid),
		Permissions: windowPermissions,
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
