// Command freeloop is the Wails desktop app: a window over the in-process
// engine.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/FinanceNik/Freeloop/frontend"
	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/platform/darwin"
)

// EventName is the Wails event every persisted session event is emitted on.
const EventName = "freeloop:event"

// ConfigEventName is emitted after config files edited on disk were
// reloaded (SET-001).
const ConfigEventName = "freeloop:config"

// OpenSessionEventName asks the UI to open a session, after a notification
// for it was clicked (GUI-008).
const OpenSessionEventName = "freeloop:open-session"

func init() {
	application.RegisterEvent[event.Event](EventName)
	application.RegisterEvent[engine.ConfigChange](ConfigEventName)
	application.RegisterEvent[string](OpenSessionEventName)
}

func main() {
	dataDir, err := config.DataDir()
	if err != nil {
		log.Fatal(err)
	}
	// FREELOOP_DEV=1 (set by `make dev`) offers the scripted demo provider.
	// Reloads can happen before the app exists; they wait here.
	configChanges := make(chan engine.ConfigChange, 16)
	notices := make(chan engine.Notice, 64)
	eng, err := engine.Open(engine.Options{
		DataDir: dataDir, Secrets: darwin.Keychain{}, Dev: os.Getenv("FREELOOP_DEV") == "1",
		OnConfigChange: func(c engine.ConfigChange) {
			select {
			case configChanges <- c:
			default: // the UI refreshes everything on any change; one is enough
			}
		},
		OnNotice: func(n engine.Notice) {
			select {
			case notices <- n:
			default: // a burst of notices; the attention view has them all
			}
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer eng.Close()

	svc := &EngineService{eng: eng}
	desk := newDesktop(eng)
	app := application.New(application.Options{
		Name:        "Freeloop",
		Description: "Run AI coding agents with your own models, harnesses, skills and environments",
		Services:    append([]application.Service{application.NewService(svc)}, desk.services()...),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontend.Assets()),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	svc.app = app

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Freeloop",
		Width:     1200,
		Height:    780,
		MinWidth:  720,
		MinHeight: 480,
		URL:       "/",
	})

	desk.start(app, win)

	// Bridge: every event the engine persists goes to the UI. Events that
	// change what needs attention also refresh the dock badge, once per
	// burst.
	stream, unsubscribe := eng.Subscribe()
	defer unsubscribe()
	badge := make(chan struct{}, 1)
	// Sessions left in error show on the badge from launch.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		select {
		case badge <- struct{}{}:
		default:
		}
	})
	go func() {
		for ev := range stream {
			app.Event.Emit(EventName, ev)
			switch ev.Type {
			case event.PermissionRequested, event.PermissionDecided, event.SessionStatus:
				select {
				case badge <- struct{}{}:
				default:
				}
			}
		}
	}()
	go func() {
		for range badge {
			time.Sleep(200 * time.Millisecond)
			desk.badge(eng.AttentionCount(context.Background()))
		}
	}()
	go func() {
		for n := range notices {
			desk.notice(n)
		}
	}()
	go func() {
		for c := range configChanges {
			app.Event.Emit(ConfigEventName, c)
		}
	}()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
