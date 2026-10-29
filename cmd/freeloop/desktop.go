//go:build !server

package main

import (
	"log"
	"strconv"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/dock"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/FinanceNik/Freeloop/internal/engine"
)

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
	d.notes.OnNotificationResponse(func(r notifications.NotificationResult) {
		if r.Error != nil {
			return
		}
		sid, _ := r.Response.UserInfo["sessionId"].(string)
		win.Show()
		win.Focus()
		if sid != "" {
			app.Event.Emit(OpenSessionEventName, sid)
		}
	})
}

func (d *desktop) notice(n engine.Notice) {
	if d.win == nil || d.win.IsFocused() {
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

func (d *desktop) badge(count int) {
	if count == 0 {
		d.dock.RemoveBadge()
		return
	}
	d.dock.SetBadge(strconv.Itoa(count))
}
